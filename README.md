# Shell-E

Shell-E is a cross-platform desktop application that provides an offline, local LLM chat interface. It uses `llama.cpp` for inference and `Fyne` for the GUI.

## Prerequisites

- **Go 1.21+**: Ensure Go is installed and in your PATH.
- **GCC**: **REQUIRED** for building the GUI (Fyne uses CGO).
  - On Windows: Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) or MSYS2. Ensure `gcc` is in your PATH.
  - On macOS: Install Xcode Command Line Tools (`xcode-select --install`).
  - On Linux: Install `gcc` and development headers for OpenGL (e.g., `libgl1-mesa-dev`, `xorg-dev`).
- **llama-cli**: You typically need the `llama-cli` executable (from [llama.cpp](https://github.com/ggerganov/llama.cpp)) available and in your PATH, or configured in settings.
- **Model**: A GGUF model file (e.g., `Llama-3-8B-Instruct.Q4_K_M.gguf`) placed in `assets/`.

## Installation

1.  **Clone the repository**:
    ```bash
    git clone https://github.com/yourusername/shell-e.git
    cd shell-e
    ```

2.  **Download dependencies**:
    ```bash
    go mod tidy
    ```

3.  **Setup Model**:
    - Place your GGUF model in the `assets/` directory (or configure the path in settings later).
    - Ensure `llama-cli` is executable and accessible.

## Building

### Windows
Ensure GCC is installed first!
```powershell
go build -o shell-e.exe ./cmd/app
```

### macOS / Linux
```bash
go build -o shell-e ./cmd/app
```

## Running

```bash
./shell-e (or ./shell-e.exe)
```

## Troubleshooting

- **"build constraints exclude all Go files"**: This usually means CGO is disabled or GCC is missing. Install GCC and ensure `CGO_ENABLED=1`.
- **Model not found**: Check `config.yaml` or settings in the app.

## License
GNU AFFERO GENERAL PUBLIC LICENSE
