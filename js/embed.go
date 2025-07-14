package js

import (
	_ "embed"
)

//go:embed Readability.js
var Readability string // https://github.com/mozilla/readability

//go:embed turndown.js
var Turndown string // https://github.com/mixmark-io/turndown

//go:embed shadowdom.js
var Shadowdom string //
