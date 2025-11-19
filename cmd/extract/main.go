package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pdflib/extractor"
	"pdflib/ir"
)

type featureSelection struct {
	Text        bool
	Images      bool
	Annotations bool
	Metadata    bool
	Bookmarks   bool
	TOC         bool
	Fonts       bool
	Attachments bool
}

type options struct {
	pdfPath  string
	outDir   string
	password string
	features featureSelection
}

func main() {
	opts, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract: %v\n", err)
		os.Exit(2)
	}
	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "extract: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() (options, error) {
	var opts options
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: go run ./cmd/extract [flags] <pdf>\n")
		flag.PrintDefaults()
	}
	text := flag.Bool("text", false, "Extract text per page")
	images := flag.Bool("images", false, "Extract image XObjects to disk")
	annotations := flag.Bool("annotations", false, "List annotations per page")
	metadata := flag.Bool("metadata", false, "Dump document metadata")
	bookmarks := flag.Bool("bookmarks", false, "Dump document outlines")
	toc := flag.Bool("toc", false, "Emit a flattened table of contents")
	fonts := flag.Bool("fonts", false, "Report font usage across pages")
	attachments := flag.Bool("attachments", false, "Extract embedded files to disk")
	outDir := flag.String("out", "extract_output", "Directory for binary artifacts (images/attachments)")
	password := flag.String("password", "", "Password to open encrypted PDFs")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		return options{}, fmt.Errorf("missing pdf path")
	}
	opts.pdfPath = flag.Arg(0)
	opts.outDir = *outDir
	opts.password = *password
	opts.features = featureSelection{
		Text:        *text,
		Images:      *images,
		Annotations: *annotations,
		Metadata:    *metadata,
		Bookmarks:   *bookmarks,
		TOC:         *toc,
		Fonts:       *fonts,
		Attachments: *attachments,
	}
	if !opts.features.Text && !opts.features.Images && !opts.features.Annotations && !opts.features.Metadata && !opts.features.Bookmarks && !opts.features.TOC && !opts.features.Fonts && !opts.features.Attachments {
		opts.features = featureSelection{Text: true, Images: true, Annotations: true, Metadata: true, Bookmarks: true, TOC: true, Fonts: true, Attachments: true}
	}
	return opts, nil
}

func run(opts options) error {
	file, err := os.Open(opts.pdfPath)
	if err != nil {
		return fmt.Errorf("open pdf: %w", err)
	}
	defer file.Close()

	pipe := ir.NewDefault()
	if opts.password != "" {
		pipe.WithPassword(opts.password)
	}
	doc, err := pipe.Parse(context.Background(), file)
	if err != nil {
		return fmt.Errorf("parse pdf: %w", err)
	}
	dec := doc.Decoded()
	if dec == nil {
		return fmt.Errorf("semantic document missing decoded backing store")
	}

	ext, err := extractor.New(dec)
	if err != nil {
		return fmt.Errorf("new extractor: %w", err)
	}

	if opts.features.Text {
		pages, err := ext.ExtractText()
		if err != nil {
			return fmt.Errorf("extract text: %w", err)
		}
		if err := emitSection("text", pages); err != nil {
			return err
		}
	}

	if opts.features.Images {
		assets, err := ext.ExtractImages()
		if err != nil {
			return fmt.Errorf("extract images: %w", err)
		}
		summaries, err := writeImages(filepath.Join(opts.outDir, "images"), assets)
		if err != nil {
			return err
		}
		if err := emitSection("images", summaries); err != nil {
			return err
		}
	}

	if opts.features.Annotations {
		annots, err := ext.ExtractAnnotations()
		if err != nil {
			return fmt.Errorf("extract annotations: %w", err)
		}
		if err := emitSection("annotations", annots); err != nil {
			return err
		}
	}

	if opts.features.Metadata {
		meta := ext.ExtractMetadata()
		if err := emitSection("metadata", meta); err != nil {
			return err
		}
	}

	if opts.features.Bookmarks {
		bookmarks := ext.ExtractBookmarks()
		if err := emitSection("bookmarks", bookmarks); err != nil {
			return err
		}
	}

	if opts.features.TOC {
		toc := ext.ExtractTableOfContents()
		if err := emitSection("table_of_contents", toc); err != nil {
			return err
		}
	}

	if opts.features.Fonts {
		fonts := ext.ExtractFonts()
		if err := emitSection("fonts", fonts); err != nil {
			return err
		}
	}

	if opts.features.Attachments {
		files := ext.ExtractEmbeddedFiles()
		summaries, err := writeAttachments(filepath.Join(opts.outDir, "attachments"), files)
		if err != nil {
			return err
		}
		if err := emitSection("attachments", summaries); err != nil {
			return err
		}
	}

	return nil
}

type imageSummary struct {
	Page         int    `json:"page"`
	ResourceName string `json:"resource"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Bits         int    `json:"bitsPerComponent"`
	ColorSpace   string `json:"colorSpace"`
	Path         string `json:"path"`
}

type attachmentSummary struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Relationship string `json:"relationship"`
	Subtype      string `json:"subtype"`
	Path         string `json:"path"`
}

func writeImages(dir string, assets []extractor.ImageAsset) ([]imageSummary, error) {
	if len(assets) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create image dir: %w", err)
	}
	summaries := make([]imageSummary, 0, len(assets))
	for idx, asset := range assets {
		name := asset.ResourceName
		if name == "" {
			name = fmt.Sprintf("img_%d", idx+1)
		}
		filename := fmt.Sprintf("page-%03d-%s.bin", asset.Page+1, safeName(name))
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, asset.Data, 0o644); err != nil {
			return nil, fmt.Errorf("write image %q: %w", path, err)
		}
		summaries = append(summaries, imageSummary{
			Page:         asset.Page,
			ResourceName: asset.ResourceName,
			Width:        asset.Width,
			Height:       asset.Height,
			Bits:         asset.BitsPerComponent,
			ColorSpace:   asset.ColorSpace,
			Path:         path,
		})
	}
	return summaries, nil
}

func writeAttachments(dir string, files []extractor.EmbeddedFile) ([]attachmentSummary, error) {
	if len(files) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create attachment dir: %w", err)
	}
	summaries := make([]attachmentSummary, 0, len(files))
	for idx, file := range files {
		name := file.Name
		if name == "" {
			name = fmt.Sprintf("attachment_%d.bin", idx+1)
		}
		path := filepath.Join(dir, safeName(name))
		if err := os.WriteFile(path, file.Data, 0o644); err != nil {
			return nil, fmt.Errorf("write attachment %q: %w", path, err)
		}
		summaries = append(summaries, attachmentSummary{
			Name:         file.Name,
			Description:  file.Description,
			Relationship: file.Relationship,
			Subtype:      file.Subtype,
			Path:         path,
		})
	}
	return summaries, nil
}

func emitSection(name string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	fmt.Printf("== %s ==\n%s\n\n", name, data)
	return nil
}

func safeName(name string) string {
	if name == "" {
		return "unnamed"
	}
	sanitized := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, name)
	return sanitized
}
