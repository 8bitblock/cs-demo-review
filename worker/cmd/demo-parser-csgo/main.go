package main

import (
	"csdemoreview/worker/internal/parser"
	"csdemoreview/worker/internal/parser/csgo"
)

func main() { parser.ChildMain("csgo", csgo.Parse) }
