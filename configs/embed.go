package configs

import _ "embed"

// Embedded so installed binaries can write a default config without the
// repository being around.
//
//go:embed lnnd.conf
var Default []byte
