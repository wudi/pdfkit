package writer

import (
	"bytes"
	"fmt"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"sort"
)

type linearizationOrder struct {
	linDict     raw.ObjectRef
	catalog     raw.ObjectRef
	page1       []raw.ObjectRef
	hintStream  raw.ObjectRef
	shared      []raw.ObjectRef
	other       []raw.ObjectRef
	mainXRef    int64
	fileLen     int64
	hintOffset  int64
	page1Offset int64
}

type linearizer struct {
	objects map[raw.ObjectRef]raw.Object
	catalog raw.ObjectRef
	info    *raw.ObjectRef
	encrypt *raw.ObjectRef

	firstPageRef raw.ObjectRef   // Added
	pageList     []raw.ObjectRef // Added

	// Classification
	page1Refs  map[raw.ObjectRef]bool
	sharedRefs map[raw.ObjectRef]bool
	otherRefs  map[raw.ObjectRef]bool

	// Per-page unique objects (for hint tables)
	// Index 0 is Page 1.
	pageObjects []map[raw.ObjectRef]bool

	// Mapping old ref -> new ref (if renumbering)
	renumber map[raw.ObjectRef]raw.ObjectRef
}

func newLinearizer(objects map[raw.ObjectRef]raw.Object, catalog raw.ObjectRef, info, encrypt *raw.ObjectRef) *linearizer {
	return &linearizer{
		objects:    objects,
		catalog:    catalog,
		info:       info,
		encrypt:    encrypt,
		page1Refs:  make(map[raw.ObjectRef]bool),
		sharedRefs: make(map[raw.ObjectRef]bool),
		otherRefs:  make(map[raw.ObjectRef]bool),
		renumber:   make(map[raw.ObjectRef]raw.ObjectRef),
	}
}

func (l *linearizer) classify() error {
	// 1. Find Page 1 and All Pages
	pagesRef, err := l.findPagesRef()
	if err != nil {
		return err
	}
	pageList, err := l.getPageList(pagesRef)
	if err != nil {
		return err
	}
	if len(pageList) == 0 {
		return fmt.Errorf("no pages found")
	}
	l.firstPageRef = pageList[0]
	l.pageList = pageList // Store page list
	l.pageObjects = make([]map[raw.ObjectRef]bool, len(pageList))

	// 2. Traverse Page 1 to find candidates
	page1Candidates := make(map[raw.ObjectRef]bool)
	l.traverse(pageList[0], page1Candidates)

	// Always include Catalog in Page 1 section (Part 4)
	page1Candidates[l.catalog] = true
	// Include Info and Encrypt if present? Usually Info is in Part 9 (Trailer) or Part 4.
	// Standard says Info is usually in Part 9, but can be in Part 4.
	// Let's put them in Shared or Other for now, or Page 1 if used.

	// 3. Traverse other pages to find shared usage
	otherUsage := make(map[raw.ObjectRef]bool)
	for i := 1; i < len(pageList); i++ {
		l.traverse(pageList[i], otherUsage)
	}

	// 4. Classify
	for ref := range page1Candidates {
		if otherUsage[ref] {
			l.sharedRefs[ref] = true
		} else {
			l.page1Refs[ref] = true
		}
	}
	l.pageObjects[0] = l.page1Refs

	// Populate pageObjects[i] for i > 0
	for i := 1; i < len(pageList); i++ {
		l.pageObjects[i] = make(map[raw.ObjectRef]bool)
		visited := make(map[raw.ObjectRef]bool)
		l.traverse(pageList[i], visited)
		for ref := range visited {
			if !l.page1Refs[ref] && !l.sharedRefs[ref] {
				l.pageObjects[i][ref] = true
			}
		}
	}

	// 5. Everything else is "Other"
	for ref := range l.objects {
		if !l.page1Refs[ref] && !l.sharedRefs[ref] {
			// Check if it was in otherUsage (it should be, unless it's unreachable or special like root)
			// If it's reachable only from other pages, it's Other.
			// If it's unreachable, we still write it as Other.
			l.otherRefs[ref] = true
		}
	}

	// Ensure Catalog is in Page 1 Refs (it was added to candidates, so if not shared, it's in page1Refs)
	// If Catalog is shared (unlikely?), it might be in sharedRefs.
	// But Catalog is the root, so it must be available for Page 1.
	// Force Catalog to be in Page 1 section if it ended up in Shared.
	if l.sharedRefs[l.catalog] {
		delete(l.sharedRefs, l.catalog)
		l.page1Refs[l.catalog] = true
	}

	return nil
}

func (l *linearizer) renumberObjects() (map[raw.ObjectRef]raw.Object, raw.ObjectRef, raw.ObjectRef, error) {
	newObjects := make(map[raw.ObjectRef]raw.Object)
	nextObj := 1

	// 1. Linearization Dict (placeholder)
	linDictRef := raw.ObjectRef{Num: nextObj, Gen: 0}
	nextObj++

	// 2. Page 1 Objects
	var p1 []raw.ObjectRef
	for ref := range l.page1Refs {
		p1 = append(p1, ref)
	}
	sort.Slice(p1, func(i, j int) bool { return p1[i].Num < p1[j].Num })

	for _, oldRef := range p1 {
		newRef := raw.ObjectRef{Num: nextObj, Gen: 0}
		l.renumber[oldRef] = newRef
		newObjects[newRef] = l.objects[oldRef]
		nextObj++
	}

	// 3. Hint Stream (placeholder)
	hintRef := raw.ObjectRef{Num: nextObj, Gen: 0}
	nextObj++

	// 4. Shared Objects
	var shared []raw.ObjectRef
	for ref := range l.sharedRefs {
		shared = append(shared, ref)
	}
	sort.Slice(shared, func(i, j int) bool { return shared[i].Num < shared[j].Num })

	for _, oldRef := range shared {
		newRef := raw.ObjectRef{Num: nextObj, Gen: 0}
		l.renumber[oldRef] = newRef
		newObjects[newRef] = l.objects[oldRef]
		nextObj++
	}

	// 5. Other Objects
	var other []raw.ObjectRef
	for ref := range l.otherRefs {
		other = append(other, ref)
	}
	sort.Slice(other, func(i, j int) bool { return other[i].Num < other[j].Num })

	for _, oldRef := range other {
		newRef := raw.ObjectRef{Num: nextObj, Gen: 0}
		l.renumber[oldRef] = newRef
		newObjects[newRef] = l.objects[oldRef]
		nextObj++
	}

	// Update references
	for ref, obj := range newObjects {
		newObjects[ref] = l.updateRefs(obj)
	}

	// Update special refs
	l.catalog = l.renumber[l.catalog]
	if l.info != nil {
		newInfo := l.renumber[*l.info]
		l.info = &newInfo
	}
	if l.encrypt != nil {
		newEnc := l.renumber[*l.encrypt]
		l.encrypt = &newEnc
	}

	return newObjects, linDictRef, hintRef, nil
}

func (l *linearizer) updateRefs(obj raw.Object) raw.Object {
	switch v := obj.(type) {
	case raw.RefObj:
		if newRef, ok := l.renumber[v.Ref()]; ok {
			return raw.Ref(newRef.Num, newRef.Gen)
		}
		return v
	case *raw.ArrayObj:
		newArr := raw.NewArray()
		for _, item := range v.Items {
			newArr.Append(l.updateRefs(item))
		}
		return newArr
	case *raw.DictObj:
		newDict := raw.Dict()
		for k, val := range v.KV {
			newDict.Set(raw.NameLiteral(k), l.updateRefs(val))
		}
		return newDict
	case *raw.StreamObj:
		newDict := l.updateRefs(v.Dict).(*raw.DictObj)
		return raw.NewStream(newDict, v.Data)
	default:
		return v
	}
}

func (l *linearizer) generateHintStream(offsets map[int]int64, lengths map[int]int64) ([]byte, error) {
	// 1. Analyze Pages
	type pageInfo struct {
		nObjects    int
		length      int64
		nShared     int
		sharedIndex int
	}
	infos := make([]pageInfo, len(l.pageList))

	// Shared objects order
	var sharedList []raw.ObjectRef
	for ref := range l.sharedRefs {
		sharedList = append(sharedList, ref)
	}
	sort.Slice(sharedList, func(i, j int) bool {
		return l.renumber[sharedList[i]].Num < l.renumber[sharedList[j]].Num
	})
	sharedIdxMap := make(map[raw.ObjectRef]int)
	for i, ref := range sharedList {
		sharedIdxMap[ref] = i
	}

	for i, pageRef := range l.pageList {
		// Objects in page
		objs := l.pageObjects[i]
		infos[i].nObjects = len(objs)

		// Length
		var length int64
		for ref := range objs {
			newRef := l.renumber[ref]
			if l, ok := lengths[newRef.Num]; ok {
				length += l
			}
		}
		infos[i].length = length

		// Shared objects
		seenShared := make(map[int]bool)
		var visit func(ref raw.ObjectRef)
		visit = func(ref raw.ObjectRef) {
			if l.sharedRefs[ref] {
				idx := sharedIdxMap[ref]
				seenShared[idx] = true
				return // Don't traverse inside shared
			}
			if !l.pageObjects[i][ref] && ref != pageRef {
				return
			}

			obj, ok := l.objects[ref]
			if !ok {
				return
			}

			refs := l.extractRefs(obj)
			for _, r := range refs {
				visit(r)
			}
		}
		visit(pageRef)

		infos[i].nShared = len(seenShared)
		minIdx := -1
		for idx := range seenShared {
			if minIdx == -1 || idx < minIdx {
				minIdx = idx
			}
		}
		if minIdx == -1 {
			minIdx = 0
		}
		infos[i].sharedIndex = minIdx
	}

	// 2. Calculate Bit Widths
	var maxNObjects, maxLength, maxNShared, maxSharedIndex int64
	for _, info := range infos {
		if int64(info.nObjects) > maxNObjects {
			maxNObjects = int64(info.nObjects)
		}
		if info.length > maxLength {
			maxLength = info.length
		}
		if int64(info.nShared) > maxNShared {
			maxNShared = int64(info.nShared)
		}
		if int64(info.sharedIndex) > maxSharedIndex {
			maxSharedIndex = int64(info.sharedIndex)
		}
	}

	bitsNObjects := bitsNeeded(maxNObjects)
	bitsLength := bitsNeeded(maxLength)
	bitsNShared := bitsNeeded(maxNShared)
	bitsSharedIndex := bitsNeeded(maxSharedIndex)

	// 3. Write Page Offset Hint Table
	var buf bytes.Buffer
	bw := newBitWriter(&buf)

	// Header
	minObjs := infos[0].nObjects
	for _, info := range infos {
		if info.nObjects < minObjs {
			minObjs = info.nObjects
		}
	}
	bw.write(uint64(minObjs), 32)

	p1Ref := l.renumber[l.firstPageRef]
	p1Offset := offsets[p1Ref.Num]
	bw.write(uint64(p1Offset), 32)

	bw.write(uint64(bitsNObjects), 16)

	minLength := infos[0].length
	for _, info := range infos {
		if info.length < minLength {
			minLength = info.length
		}
	}
	bw.write(uint64(minLength), 32)

	bw.write(uint64(bitsLength), 16)
	bw.write(0, 32) // Content stream offset
	bw.write(0, 16) // Bits for content stream offset
	bw.write(0, 32) // Content stream length
	bw.write(0, 16) // Bits for content stream length
	bw.write(uint64(bitsNShared), 16)
	bw.write(uint64(bitsSharedIndex), 16)
	bw.write(0, 16) // Numerator
	bw.write(0, 16) // Denominator

	// Entries
	for _, info := range infos {
		bw.write(uint64(info.nObjects-minObjs), uint(bitsNObjects))
		bw.write(uint64(info.length-minLength), uint(bitsLength))
		bw.write(uint64(info.nShared), uint(bitsNShared))
		bw.write(uint64(info.sharedIndex), uint(bitsSharedIndex))
	}

	bw.flush()

	// 4. Shared Object Hint Table
	// Header
	firstSharedOffset := int64(0)
	if len(sharedList) > 0 {
		firstSharedOffset = offsets[l.renumber[sharedList[0]].Num]
	}
	bw.write(uint64(firstSharedOffset), 32)
	bw.write(0, 32) // Location of first shared object hint table entry

	maxSharedLen := int64(0)
	for _, ref := range sharedList {
		newRef := l.renumber[ref]
		if l, ok := lengths[newRef.Num]; ok {
			if l > maxSharedLen {
				maxSharedLen = l
			}
		}
	}
	bitsSharedLen := bitsNeeded(maxSharedLen)
	bw.write(uint64(bitsSharedLen), 16)
	bw.write(0, 16) // Signature

	// Entries
	for _, ref := range sharedList {
		newRef := l.renumber[ref]
		l := lengths[newRef.Num]
		bw.write(uint64(l), uint(bitsSharedLen))
	}

	bw.flush()

	return buf.Bytes(), nil
}

func bitsNeeded(val int64) int {
	if val == 0 {
		return 0
	}
	bits := 0
	for val > 0 {
		bits++
		val >>= 1
	}
	return bits
}

type bitWriter struct {
	buf         *bytes.Buffer
	accumulator uint64
	bits        uint
}

func newBitWriter(buf *bytes.Buffer) *bitWriter {
	return &bitWriter{buf: buf}
}

func (w *bitWriter) write(val uint64, n uint) {
	if n == 0 {
		return
	}
	w.accumulator = (w.accumulator << n) | (val & ((1 << n) - 1))
	w.bits += n
	for w.bits >= 8 {
		w.bits -= 8
		w.buf.WriteByte(byte(w.accumulator >> w.bits))
	}
}

func (w *bitWriter) flush() {
	if w.bits > 0 {
		w.accumulator <<= (8 - w.bits)
		w.buf.WriteByte(byte(w.accumulator))
		w.bits = 0
		w.accumulator = 0
	}
}

func (l *linearizer) findPagesRef() (raw.ObjectRef, error) {
	catObj, ok := l.objects[l.catalog]
	if !ok {
		return raw.ObjectRef{}, fmt.Errorf("catalog missing")
	}
	catDict, ok := catObj.(*raw.DictObj)
	if !ok {
		return raw.ObjectRef{}, fmt.Errorf("catalog not a dict")
	}
	pagesObj, ok := catDict.Get(raw.NameLiteral("Pages"))
	if !ok {
		return raw.ObjectRef{}, fmt.Errorf("Pages missing in catalog")
	}
	if ref, ok := pagesObj.(raw.RefObj); ok {
		return ref.Ref(), nil
	}
	return raw.ObjectRef{}, fmt.Errorf("Pages not a ref")
}

func (l *linearizer) getPageList(pagesRef raw.ObjectRef) ([]raw.ObjectRef, error) {
	// Simple traversal of the page tree
	var list []raw.ObjectRef
	var visit func(ref raw.ObjectRef) error
	visit = func(ref raw.ObjectRef) error {
		obj, ok := l.objects[ref]
		if !ok {
			return nil // Missing object
		}
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			return nil
		}
		typ, ok := dict.Get(raw.NameLiteral("Type"))
		if !ok {
			return nil
		}
		name, ok := typ.(raw.NameObj)
		if !ok {
			return nil
		}
		if name.Value() == "Page" {
			list = append(list, ref)
			return nil
		}
		if name.Value() == "Pages" {
			kids, ok := dict.Get(raw.NameLiteral("Kids"))
			if !ok {
				return nil
			}
			arr, ok := kids.(*raw.ArrayObj)
			if !ok {
				return nil
			}
			for _, item := range arr.Items {
				if kRef, ok := item.(raw.RefObj); ok {
					if err := visit(kRef.Ref()); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := visit(pagesRef); err != nil {
		return nil, err
	}
	return list, nil
}

func (l *linearizer) traverse(root raw.ObjectRef, visited map[raw.ObjectRef]bool) {
	if visited[root] {
		return
	}
	visited[root] = true
	obj, ok := l.objects[root]
	if !ok {
		return
	}

	// Find all refs in this object
	refs := l.extractRefs(obj)
	for _, r := range refs {
		l.traverse(r, visited)
	}
}

func (l *linearizer) extractRefs(obj raw.Object) []raw.ObjectRef {
	var refs []raw.ObjectRef
	switch v := obj.(type) {
	case raw.RefObj:
		refs = append(refs, v.Ref())
	case *raw.ArrayObj:
		for _, item := range v.Items {
			refs = append(refs, l.extractRefs(item)...)
		}
	case *raw.DictObj:
		for _, val := range v.KV {
			refs = append(refs, l.extractRefs(val)...)
		}
	case *raw.StreamObj:
		refs = append(refs, l.extractRefs(v.Dict)...)
		// Stream data is opaque, no refs inside
	}
	return refs
}

func (w *impl) writeLinearized(ctx Context, doc *semantic.Document, out WriterAt, cfg Config) error {
	// 1. Build objects
	builder := newObjectBuilder(doc, cfg, 1)
	objects, catalogRef, infoRef, encryptRef, err := builder.Build()
	if err != nil {
		return err
	}

	// 2. Linearize
	l := newLinearizer(objects, catalogRef, infoRef, encryptRef)
	if err := l.classify(); err != nil {
		return err
	}
	newObjects, linDictRef, hintRef, err := l.renumberObjects()
	if err != nil {
		return err
	}
	idPair := fileID(doc, cfg)

	// 3. Prepare Linearization Dict and Hint Stream
	linDict := raw.Dict()
	linDict.Set(raw.NameLiteral("Linearized"), raw.NumberInt(1))
	linDict.Set(raw.NameLiteral("L"), raw.NumberInt(0))
	linDict.Set(raw.NameLiteral("H"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0)))
	linDict.Set(raw.NameLiteral("O"), raw.NumberInt(int64(l.renumber[l.firstPageRef].Num)))
	linDict.Set(raw.NameLiteral("E"), raw.NumberInt(0))
	linDict.Set(raw.NameLiteral("N"), raw.NumberInt(int64(len(doc.Pages))))
	linDict.Set(raw.NameLiteral("T"), raw.NumberInt(0))

	newObjects[linDictRef] = linDict

	// Reserve space for hint stream
	hintStream := raw.NewStream(raw.Dict(), make([]byte, 4096))
	hintStream.Dict.Set(raw.NameLiteral("S"), raw.NumberInt(0))
	newObjects[hintRef] = hintStream

	// 4. Calculate Sizes & Offsets
	sortedRefs := make([]raw.ObjectRef, 0, len(newObjects))
	for ref := range newObjects {
		sortedRefs = append(sortedRefs, ref)
	}
	sort.Slice(sortedRefs, func(i, j int) bool { return sortedRefs[i].Num < sortedRefs[j].Num })

	lengths := make(map[int]int64)
	offsets := make(map[int]int64)

	// Find max Page 1 obj num
	maxP1Num := 1 // LinDict
	for ref := range l.page1Refs {
		if n := l.renumber[ref].Num; n > maxP1Num {
			maxP1Num = n
		}
	}

	version := pdfVersion(cfg)
	header := []byte("%PDF-" + version + "\n%\xE2\xE3\xCF\xD3\n")
	headerLen := int64(len(header))

	// Iterative sizing
	for pass := 0; pass < 3; pass++ {
		// Serialize all to get lengths
		for _, ref := range sortedRefs {
			data, _ := w.SerializeObject(ref, newObjects[ref])
			lengths[ref.Num] = int64(len(data))
		}

		// Calculate offsets
		currentOffset := headerLen

		// Obj 1 (LinDict)
		offsets[linDictRef.Num] = currentOffset
		currentOffset += lengths[linDictRef.Num]

		// First Page XRef
		// We need to simulate its size.
		// It covers 0 to maxP1Num.
		// We can build it to a buffer.
		// Note: We need offsets for it to be correct, but for size estimation,
		// we can use current offsets (which might be slightly off, but XRef entries are fixed size 20 bytes).
		// Wait, XRef entries are fixed size! 20 bytes.
		// So size depends only on count.
		// But the Trailer is part of it. Trailer has "Size".
		// Trailer size is variable.

		// Let's build a dummy trailer to measure size.
		fpTrailer := buildTrailer(maxP1Num+1, raw.ObjectRef{}, nil, nil, doc, cfg, 0, idPair)
		fpTrailerBytes := serializePrimitive(fpTrailer)

		// xref\n0 N\n...
		xrefLen := int64(5 + len(fmt.Sprintf("0 %d\n", maxP1Num+1)) + (maxP1Num+1)*20)
		xrefLen += int64(len("trailer\n") + len(fpTrailerBytes) + 1)

		// First Page XRef doesn't have startxref?
		// "The first page trailer... shall not contain a Prev entry... shall not contain a startxref..."
		// So just trailer.

		currentOffset += xrefLen

		// Page 1 Objects
		for _, ref := range sortedRefs {
			if ref.Num > 1 && ref.Num <= maxP1Num {
				offsets[ref.Num] = currentOffset
				currentOffset += lengths[ref.Num]
			}
		}

		// Hint Stream (Obj maxP1Num + 1)
		offsets[hintRef.Num] = currentOffset
		currentOffset += lengths[hintRef.Num]

		// Shared & Other
		for _, ref := range sortedRefs {
			if ref.Num > hintRef.Num {
				offsets[ref.Num] = currentOffset
				currentOffset += lengths[ref.Num]
			}
		}

		fileLen := currentOffset
		maxObjNum := sortedRefs[len(sortedRefs)-1].Num
		size := maxObjNum + 1
		entryCount := size - (maxP1Num + 1)
		if entryCount < 0 {
			entryCount = 0
		}
		fpXRefOffset := offsets[linDictRef.Num] + lengths[linDictRef.Num]
		mainTrailer := buildTrailer(size, l.catalog, l.info, l.encrypt, doc, cfg, 0, idPair)
		mainTrailer.Set(raw.NameLiteral("Prev"), raw.NumberInt(fpXRefOffset))
		trailerBytes := serializePrimitive(mainTrailer)
		mainXRefLen := int64(len("xref\n"))
		mainXRefLen += int64(len(fmt.Sprintf("%d %d\n", maxP1Num+1, entryCount)))
		mainXRefLen += int64(entryCount) * 20
		mainXRefLen += int64(len("trailer\n"))
		mainXRefLen += int64(len(trailerBytes))
		mainXRefLen += int64(len("\nstartxref\n"))
		mainXRefLen += int64(len(fmt.Sprintf("%d\n%%EOF\n", currentOffset)))
		fileLen += mainXRefLen

		// Generate Hint Stream
		hintData, err := l.generateHintStream(offsets, lengths)
		if err != nil {
			return err
		}

		// Update Hint Stream
		hintStream.Data = hintData

		// Update LinDict
		linDict.Set(raw.NameLiteral("L"), raw.NumberInt(fileLen))
		linDict.Set(raw.NameLiteral("H"), raw.NewArray(
			raw.NumberInt(offsets[hintRef.Num]),
			raw.NumberInt(lengths[hintRef.Num]),
		))
		linDict.Set(raw.NameLiteral("E"), raw.NumberInt(offsets[hintRef.Num]))

		// Main XRef Offset (T)
		// Main XRef starts at fileLen.
		linDict.Set(raw.NameLiteral("T"), raw.NumberInt(currentOffset))
	}

	// 5. Final Write
	var buf bytes.Buffer
	buf.Write(header)

	// Obj 1
	serialized, _ := w.SerializeObject(linDictRef, newObjects[linDictRef])
	buf.Write(serialized)

	// First Page XRef
	buf.WriteString("xref\n")
	buf.WriteString(fmt.Sprintf("0 %d\n", maxP1Num+1))
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= maxP1Num; i++ {
		if off, ok := offsets[i]; ok {
			buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
		} else {
			buf.WriteString("0000000000 65535 f \n")
		}
	}
	fpTrailer := buildTrailer(maxP1Num+1, raw.ObjectRef{}, nil, nil, doc, cfg, 0, idPair)
	buf.WriteString("trailer\n")
	buf.Write(serializePrimitive(fpTrailer))
	buf.WriteString("\n")

	// Page 1 Objects
	for _, ref := range sortedRefs {
		if ref.Num > 1 && ref.Num <= maxP1Num {
			serialized, _ := w.SerializeObject(ref, newObjects[ref])
			buf.Write(serialized)
		}
	}

	// Hint Stream
	serialized, _ = w.SerializeObject(hintRef, newObjects[hintRef])
	buf.Write(serialized)

	// Shared & Other
	for _, ref := range sortedRefs {
		if ref.Num > hintRef.Num {
			serialized, _ := w.SerializeObject(ref, newObjects[ref])
			buf.Write(serialized)
		}
	}

	// Main XRef
	mainXRefOffset := int64(buf.Len())
	maxObjNum := sortedRefs[len(sortedRefs)-1].Num
	size := maxObjNum + 1

	buf.WriteString("xref\n")
	// We can write just the update (from maxP1Num+1 to end) or full table.
	// Linearized PDF usually has Main XRef starting from 0 but with some entries free?
	// Or just the new entries.
	// "The main cross-reference table... shall consist of entries for all objects... that are not listed in the first page cross-reference table."
	// So it starts at maxP1Num + 1?
	// Yes, usually.

	buf.WriteString(fmt.Sprintf("%d %d\n", maxP1Num+1, size-(maxP1Num+1)))
	for i := maxP1Num + 1; i < size; i++ {
		if off, ok := offsets[i]; ok {
			buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
		} else {
			buf.WriteString("0000000000 65535 f \n")
		}
	}

	trailer := buildTrailer(size, l.catalog, l.info, l.encrypt, doc, cfg, 0, idPair)
	// Main trailer must have Prev pointing to First Page XRef?
	// "The trailer... shall contain a Prev entry giving the offset of the first page cross-reference table."
	// First Page XRef offset is offsets[linDictRef.Num] + lengths[linDictRef.Num]
	fpXRefOffset := offsets[linDictRef.Num] + lengths[linDictRef.Num]
	trailer.Set(raw.NameLiteral("Prev"), raw.NumberInt(fpXRefOffset))

	buf.WriteString("trailer\n")
	buf.Write(serializePrimitive(trailer))
	buf.WriteString("\nstartxref\n")
	buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", mainXRefOffset))

	_, err = out.Write(buf.Bytes())
	return err
}
