项目目标**零妥协、包含所有规范**，甚至包含 PDF/A 系列，不再仅仅是“解析 PDF 文件”那么简单，而是涉及**PDF 标准家族（The PDF Family of Standards）**的完整实现。

要实现一个真正全面的库，你的 Parser 必须能向后兼容 30 年前的 PDF 1.0，而你的 Creator 必须能生成符合 ISO 严苛标准的 PDF/A、PDF/X 等格式。

这份列表基于 **ISO 32000-1 (PDF 1.7)**, **ISO 32000-2 (PDF 2.0)** 以及 **ISO 19005 (PDF/A 系列)** 等标准文件整理。

---

### 第一板块：核心架构与语法 (Core Syntax & File Structure)
*这部分是所有 PDF 的基础，Parser 必须全支持，Creator 需选择版本支持。*

#### 1. 文件物理结构
- [x] **Header**: 支持从 `%PDF-1.0` 到 `%PDF-2.0` 所有版本头。
- [x] **Trailer Dictionary**: Root, Encrypt, Info, ID, Previous (用于增量更新)。
- [x] **Cross-Reference Table (xref)**:
    - [x] Classic xref table (纯文本表格).
    - [x] **XRef Streams**: 压缩的交叉引用流 (PDF 1.5+).
    - [x] **Hybrid Reference**: 混合使用传统表和流 (常见于过渡期文件).
- [x] **Incremental Updates**: 能够读取和写入追加到文件末尾的修改量，保持原始签名有效。
- [ ] **Linearization (Fast Web View)**: 支持 Hint Tables，允许字节流式加载。
- [x] **Object Streams**: 解析和生成压缩的对象流 (ObjStm)。

#### 2. 基础对象 (Cos Objects)
- [x] **Null, Boolean, Integer, Real**.
- [x] **String**:
    - [x] Literal Strings (包含八进制转义 `\ddd` 和平衡括号).
    - [x] Hex Strings (十六进制 `<...>`).
    - [x] UTF-16BE Strings (BOM 开头).
- [x] **Names**: 正确处理 `/` 后面的转义序列 (如 `#20`).
- [x] **Arrays & Dictionaries**.
- [x] **Streams**: 处理 `Length` 为间接对象的情况，处理外部文件引用 (F 键)。

#### 3. 过滤器与压缩 (Filters)
*全集必须包含以下所有，包括已过时的。*
- [x] **FlateDecode**: (Zlib/Deflate) 支持 Predictor functions (PNG predictors).
- [x] **LZWDecode**: 支持 EarlyChange 参数。
- [x] **ASCII85Decode** & **ASCIIHexDecode**.
- [x] **RunLengthDecode**.
- [x] **CCITTFaxDecode**: Group 3 (1D/2D) 和 Group 4。
- [x] **JBIG2Decode**: 处理嵌入的 Global Segments。
- [x] **DCTDecode**: JPEG 图像处理。
- [x] **JPXDecode**: JPEG 2000 (PDF 1.5+)。
- [x] **Crypt**: 专门用于解密流的过滤器。

---

### 第二板块：图形与渲染模型 (Graphics & Imaging)
*这是 ISO 32000 的核心。*

#### 1. 颜色空间 (Color Spaces)
- [ ] **Device Spaces**: DeviceGray, DeviceRGB, DeviceCMYK.
- [ ] **CIE-based Spaces**: CalGray, CalRGB, Lab, ICCBased (完整解析 ICC Profile)。
- [ ] **Special Spaces**:
    - [ ] **Indexed**: 索引颜色。
    - [ ] **Pattern**: 能够绘制平铺或渐变作为颜色。
    - [ ] **Separation**: 专色 (Spot Colors)。
    - [ ] **DeviceN**: 任意数量的专色混合 (包含 NChannel)。

#### 2. 图案与底纹 (Patterns & Shading)
- [ ] **Tiling Patterns**: Type 1 (Colored & Uncolored)。
- [ ] **Shading Patterns (Type 2)**:
    - [ ] Type 1: Function-based.
    - [ ] Type 2: Axial (线性渐变).
    - [ ] Type 3: Radial (径向渐变).
    - [ ] Type 4: Free-form Gouraud-shaded triangle meshes.
    - [ ] Type 5: Lattice-form Gouraud-shaded triangle meshes.
    - [ ] Type 6: Coons patch meshes.
    - [ ] Type 7: Tensor-product patch meshes (最复杂的曲面渐变).

#### 3. 图像处理 (XObject Images)
- [ ] **Image Dictionary**: Width, Height, BitsPerComponent, ImageMask, Mask (Color Key & Stencil Masking), Decode (反色), Interpolate.
- [ ] **SMask (Soft Mask)**: 图像的透明度通道（作为另一个 Image XObject 链接）。
- [ ] **Inline Images**: 内容流中的 `BI...ID...EI` 块。
- [ ] **Alternates**: 为打印或低分屏提供不同版本的图像。

#### 4. 透明度模型 (Transparency)
- [ ] **Graphics State Parameters**: CA/ca (Alpha), BM (Blend Mode).
- [ ] **Blend Modes**: 必须实现所有 16 种混合模式 (Normal, Multiply, Screen, Overlay, Darken, Lighten, ColorDodge, ColorBurn, HardLight, SoftLight, Difference, Exclusion, Hue, Saturation, Color, Luminosity)。
- [ ] **Transparency Groups**: Isolated (隔离组), Knockout (挖空组)。
- [ ] **Soft Masks**: Luminosity 和 Alpha 类型的蒙版组。

---

### 第三板块：字体与文本 (Fonts & Text)
*PDF 最复杂的深水区。*

#### 1. 字体格式支持
- [ ] **Type 1 (PostScript)**: 解析 .pfb/.pfm 数据。
- [ ] **Type 3**: 由 PDF 图形操作符构成的自定义字形。
- [x] **TrueType**: 解析与提取。
- [ ] **Type 0 (Composite Fonts)**: CID-Keyed Fonts (用于 CJK 支持)。
- [ ] **OpenType**:
    - [ ] CFF (Compact Font Format) outlines.
    - [ ] TrueType outlines.
- [ ] **OCF (Original Composite Format)**: 极老版本的中文支持 (Parser 需兼容)。

#### 2. 编码与映射
- [ ] **Simple Encodings**: WinAnsi, MacRoman, StandardEncoding.
- [ ] **Built-in CMaps**: Identity-H/V, GBK-EUC, UniJIS 等标准 CMap。
- [ ] **ToUnicode CMap**: **关键**，用于从字形 ID 反向提取 Unicode 文本。
- [ ] **Font Descriptor**: 字体度量 (Ascent, Descent, CapHeight, Flags)。

---

### 第四板块：交互性与多媒体 (Interactivity)

#### 1. 注释全集 (Annotations)
*ISO 32000-2 定义的全部类型：*
- [ ] Text (Sticky Note), Link, FreeText, Line, Square, Circle, Polygon, PolyLine.
- [ ] Highlight, Underline, Squiggly, StrikeOut (文本标记).
- [ ] Stamp (图章), Caret, Ink (手写).
- [ ] Popup (弹出窗口), FileAttachment.
- [ ] Sound, Movie (旧版), Screen (新版媒体), Widget (表单).
- [ ] PrinterMark, TrapNet (印刷标记).
- [ ] Watermark.
- [ ] 3D (U3D & PRC 格式).
- [ ] **Redact**: 脱敏注释（不仅仅是画黑框，还要能物理删除底层内容）。
- [ ] **Projection**: (PDF 2.0 新增) 用于投影内容的注释。

#### 2. 动作 (Actions)
- [ ] GoTo, GoToR (Remote), GoToE (Embedded).
- [ ] Launch, Thread, URI.
- [ ] Sound, Movie, Hide.
- [ ] Named (NextPage, PrevPage, etc.).
- [ ] SubmitForm, ResetForm, ImportData.
- [ ] **JavaScript**: 支持 Acrobat JavaScript API (需要集成 JS 引擎如 SpiderMonkey 或 V8)。
- [ ] Trans (页面切换效果)。
- [ ] GoTo3DView, RichMediaExecute.

---

### 第五板块：表单技术 (Forms)
*你需要支持三代表单技术。*

1.  **AcroForms (基于 Widget 注释)**:
    - [x] Fields: Text, Button (Push/Check/Radio), Choice (Combo/List), Signature.
    - [ ] NeedAppearances: 能够根据值自动生成外观流 (Appearance Stream)。
    - [ ] Calculation Order: 字段间计算依赖。

2.  **XFA (XML Forms Architecture)**:
    - [ ] **注意**: 虽然 PDF 2.0 已弃用 XFA，但全能 Parser **必须**能读取它（从 XFA 字典提取 XML），否则无法处理大量政府/企业历史文档。
    - [ ] Dynamic XFA Rendering: 解析 XML 并渲染布局（极高难度）。

3.  **HTML Forms (Acrobat DC 新特性)**:
    - [ ] 能够识别嵌入的 Web 内容形式。

---

### 第六板块：合规性标准 (Compliance & Archiving) - **重点要求**
*这里是“特性”变成“约束”的地方。包含 Creator 的验证逻辑。*

#### 1. ISO 19005 (PDF/A) - 长期归档
*Creator 必须能将普通 PDF 转换为符合以下标准的 PDF：*
- [x] **PDF/A-1 (Based on PDF 1.4)**:
    - [x] 禁止透明度 (Transparency)。
    - [x] 禁止加密。
    - [x] 所有字体必须嵌入。
    - [x] 颜色必须是 DeviceIndependent 或有 OutputIntent。
    - [x] 禁止 JavaScript 和可执行 Launch 动作。
    - [ ] 区分 Level A (Accessible, 严格标签) 和 Level B (Basic, 视觉一致)。
- [ ] **PDF/A-2 (Based on PDF 1.7)**:
    - [ ] 允许透明度。
    - [ ] 允许 JPEG2000。
    - [ ] 允许嵌入 PDF/A 文件。
    - [ ] 包含 Level A, B, U (Unicode 映射)。
- [ ] **PDF/A-3**:
    - [ ] 允许嵌入**任意**文件格式 (作为附件)，只要主文件符合 A-2。
- [ ] **PDF/A-4 (Based on PDF 2.0)**:
    - [ ] 移除 Level A/B/U 分级，改为基本级、F (File 附件) 和 E (Engineering)。

#### 2. ISO 15930 (PDF/X) - 图形交换/印刷
- [ ] 强制 OutputIntent (ICC Profile)。
- [ ] 限制颜色空间 (通常仅 CMYK + Spot)。
- [ ] 必须包含 TrimBox/BleedBox。
- [ ] 禁止注解位于打印区域内。

#### 3. ISO 14289 (PDF/UA) - 通用辅助功能
- [ ] 强制 **Tagged PDF** (结构化标签树)。
- [ ] 所有非文本内容必须有 Alt Text。
- [ ] 标题层级结构必须逻辑正确。
- [ ] 字体必须包含 Unicode 映射。

#### 4. ISO 24517 (PDF/E) - 工程图纸
- [ ] 支持 Geo (地理空间) 和 3D 内容的特定约束。
- [ ] 支持超大页面尺寸 (超过 200 英寸)。

#### 5. ISO 16612 (PDF/VT) - 可变数据打印
- [ ] 优化重复资源 (XObject) 的缓存。
- [ ] 支持 DPart (Document Part) 元数据层级。

---

### 第七板块：安全与加密 (Security)

- [x] **User/Owner Passwords**: 权限位控制 (Print, Copy, Modify)。
- [x] **Encryption Algorithms**:
    - [x] RC4 (40, 128 bit) - 旧版兼容。
    - [x] AES (128, 256 bit) - 现代标准。
    - [ ] **PDF 2.0 Encryption**: 仅支持 AES-256，移除 RC4。
- [ ] **Unencrypted Wrapper Document**: PDF 2.0 特性，允许文件封套不加密，内部 Payload 加密。
- [ ] **Digital Signatures**:
    - [x] PKCS#1, PKCS#7, CMS, CAdES.
    - [ ] **PAdES** (PDF Advanced Electronic Signatures) 兼容性 (ETSI 标准)。
    - [ ] **DocTimeStamp**: RFC 3161 时间戳支持。
    - [ ] **LTV (Long Term Validation)**: 嵌入 DSS (Document Security Store) 包含证书链和 OCSP/CRL 响应。

---

### 第八板块：逻辑结构与元数据 (Structure & Metadata)

- [x] **Document Info Dictionary**: (Title, Author 等) 在 PDF 2.0 中已弃用，但 Parser 需读取。
- [x] **XMP Metadata**: 必须支持解析嵌入在 Stream 中的 XML 元数据包。
- [ ] **Tagged PDF (Logical Structure)**:
    - [ ] StructTreeRoot, StructElem.
    - [ ] Standard Structure Types (P, H1, Table, Figure, etc.).
    - [ ] **RoleMap**: 自定义标签映射。
    - [ ] **PDF 2.0 Namespaces**: 支持多命名空间标签 (如 MathML)。
- [ ] **Associated Files (AF)**: PDF 2.0/A-3 特性，文件级或对象级的文件关联。
- [ ] **GeoSpatial**: GeoPDF 特性，视口与地理坐标的映射 (Lat/Lon)。

---

### 第九板块：PDF 2.0 独有新特性 (ISO 32000-2 Specific)
*如果你的库声称支持“最新”，必须包含这些：*

- [ ] **Page-level Output Intents**: 页面级输出意图。
- [ ] **Black Point Compensation**: 图形状态参数。
- [ ] **Halftones**: 任意半色调控制。
- [ ] **Spectrally Defined Colors**: 基于光谱数据的颜色定义 (CxF)。
- [ ] **GoToDP / GoTo3DView**: 具体的 3D/导航动作更新。
- [ ] **RichMedia Annotations 的废弃**: 转而使用 Screen Annotations。

---

### 验证与测试策略

要验证你的“全面”实现，你需要收集或生成以下测试集：
1.  **Isartor Test Suite**: 专门用于验证 PDF/A-1b 合规性的官方测试集。
2.  **VeraPDF Corpus**: 欧盟资助的项目，用于验证 PDF/A 所有版本的验证器。
3.  **Ghent Output Suite**: 用于验证 PDF/X 和印刷渲染准确性。
4.  **PDF Association "PDF 2.0 Examples"**: 验证 PDF 2.0 新特性。

建议先完成 ISO 32000-1 (PDF 1.7) 的核心部分，再逐步攻克 PDF/A 和 PDF 2.0。