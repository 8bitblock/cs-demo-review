package main

import (
	"csdemoreview/worker/internal/parser"
	"csdemoreview/worker/internal/parser/cs2"
)

func main() { parser.ChildMain("cs2", cs2.Parse) }
