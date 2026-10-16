// Package web embeds the public landing page and the built operator
// console (React/TypeScript M2 bundle in console-dist/, prebuilt so a
// fresh clone serves it without Node).
package web

import "embed"

//go:embed landing.html landing.css landing.js
var FS embed.FS

//go:embed all:console-dist
var Dist embed.FS
