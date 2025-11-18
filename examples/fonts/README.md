# Embedded Fonts Example

This sample shows how to embed a TrueType font from `testdata/*.TTF` and draw the Chinese greeting "你好，世界！" using the fluent PDF builder.

It demonstrates:

- loading a UTF-8 string into the document via a registered TrueType font
- registering the font once and reusing it through standard builder text operations
- writing the PDF with the default writer configuration

## Download fonts
Download these fonts and place them in the `testdata` directory.

 - [SimHei-Regular.ttf](https://freefonts.co/fonts/simhei-regular)
 - [Rubik-Regular.ttf](https://fonts.google.com/specimen/Rubik)
 - [NotoSansJP-Regular.ttf](https://fonts.google.com/noto/specimen/Noto+Sans+JP)

## Run It

```bash
go run ./examples/fonts [output.pdf]
```

The program defaults to `fonts.pdf` in the current directory.
