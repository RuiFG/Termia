package embedded

import _ "embed"

//go:embed schema.sql
var SchemaSQL string

//go:embed termia.zsh
var TermiaZsh string

//go:embed termia.bash
var TermiaBash string

//go:embed termia.zshrc
var TermiaZshRC string

//go:embed termia.bashrc
var TermiaBashRC string

//go:embed config.toml
var DefaultConfig string
