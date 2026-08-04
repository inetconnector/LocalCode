# Git setup

This source package is ready to be opened in Visual Studio and committed to Git.

## First commit

From a terminal in this directory:

```powershell
git init
git add .
git commit -F COMMIT_MESSAGE.txt
```

`COMMIT_MESSAGE.txt` contains the suggested commit message.

## Included repository files

- `.gitignore`: Visual Studio, Go, build outputs, local runtime state, logs and secrets.
- `.gitattributes`: consistent line endings for Go, documentation and Windows scripts.
- `.editorconfig`: formatting defaults understood by Visual Studio and many other editors.
- `COMMIT_MESSAGE.txt`: ready-to-use commit message.

Generated executables and the `dist` directory are intentionally ignored. Rebuild them with
`BUILD-AND-RUN.bat` or `BUILD.bat`.
