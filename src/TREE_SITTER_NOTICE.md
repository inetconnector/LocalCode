# Tree-sitter dependencies

LocalCode embeds the official Go Tree-sitter bindings and selected language grammars for deterministic repository intelligence when CGO is enabled.

Current direct dependencies:

- `github.com/tree-sitter/go-tree-sitter`
- `github.com/tree-sitter/tree-sitter-javascript`
- `github.com/tree-sitter/tree-sitter-typescript`
- `github.com/tree-sitter/tree-sitter-python`
- `github.com/tree-sitter/tree-sitter-rust`
- `github.com/tree-sitter/tree-sitter-c`
- `github.com/tree-sitter/tree-sitter-cpp`

These upstream projects are maintained under the `tree-sitter` GitHub organization and are distributed under the MIT License. Their copyright and license files remain authoritative for the corresponding dependency versions resolved by Go modules.

LocalCode's own source remains licensed under Apache-2.0. This notice does not change the upstream licenses.
