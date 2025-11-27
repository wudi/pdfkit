# Tesseract OCR Engine Integration

## Install Tesseract OCR

### MacOS:
```bash
brew install tesseract
brew install leptonica

export CPATH=/opt/homebrew/include
export LIBRARY_PATH=/opt/homebrew/lib
```

### Ubuntu/Debian:
```bash
sudo apt update
sudo apt install tesseract-ocr
sudo apt install libleptonica-dev
```

### Other Linux:
Please refer to the [Tesseract OCR installation guide](https://tesseract-ocr.github.io/tessdoc/Installation.html) for instructions specific to your distribution.