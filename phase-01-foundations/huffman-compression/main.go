// Command huffman is a lossless file compressor using Huffman coding.
//
// Usage:
//
//	huffman compress   [-o out] <input>
//	huffman decompress [-o out] <input>
//	huffman c ...   (alias)   huffman d ...   (alias)
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  domain failure (corrupt input, decode error)
//	2  usage / IO error (bad args, file not found)
package main

import (
	"fmt"
	"os"

	"huffman/internal/huffman"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}

	cmd := args[0]
	rest := args[1:]

	// Tiny flag parse: optional "-o <out>" then a required input path.
	var outPath string
	var inPath string
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "-o", "--output":
			if i+1 >= len(rest) {
				fmt.Fprintln(os.Stderr, "error: -o requires a value")
				return 2
			}
			outPath = rest[i+1]
			i++
		default:
			if inPath != "" {
				fmt.Fprintf(os.Stderr, "error: unexpected argument %q\n", rest[i])
				return 2
			}
			inPath = rest[i]
		}
	}

	switch cmd {
	case "compress", "c":
		return doCompress(inPath, outPath)
	case "decompress", "d":
		return doDecompress(inPath, outPath)
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n", cmd)
		usage()
		return 2
	}
}

func doCompress(inPath, outPath string) int {
	if inPath == "" {
		fmt.Fprintln(os.Stderr, "error: missing input file")
		return 2
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	if outPath == "" {
		outPath = inPath + ".huf"
	}
	encoded, err := huffman.Compress(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := os.WriteFile(outPath, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	ratio := huffman.Ratio(len(data), len(encoded))
	fmt.Printf("compressed %s (%d bytes) -> %s (%d bytes)\n", inPath, len(data), outPath, len(encoded))
	if len(data) > 0 {
		fmt.Printf("compression ratio: %.3f (%.1f%% of original, saved %.1f%%)\n",
			ratio, ratio*100, (1-ratio)*100)
	}
	return 0
}

func doDecompress(inPath, outPath string) int {
	if inPath == "" {
		fmt.Fprintln(os.Stderr, "error: missing input file")
		return 2
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	if outPath == "" {
		fmt.Fprintln(os.Stderr, "error: -o <output> is required for decompress")
		return 2
	}
	decoded, err := huffman.Decompress(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := os.WriteFile(outPath, decoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	fmt.Printf("decompressed %s (%d bytes) -> %s (%d bytes)\n", inPath, len(data), outPath, len(decoded))
	return 0
}

func usage() {
	fmt.Fprint(os.Stderr, `huffman — lossless Huffman file compressor

Usage:
  huffman compress   [-o output] <input>     # alias: c   (default output: <input>.huf)
  huffman decompress  -o output  <input>     # alias: d

Examples:
  huffman compress   -o book.huf book.txt
  huffman decompress -o book.txt book.huf
`)
}
