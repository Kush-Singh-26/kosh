package base

import _ "embed"

// BaseTemplate contains the embedded base HTML template.
//
//go:embed base.html
var BaseTemplate string
