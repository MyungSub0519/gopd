package gopd_test

import (
	"fmt"
	"os"

	"github.com/MyungSub0519/gopd"
)

func ExampleParseFile() {
	result, err := gopd.ParseFile("testdata/synthetic.pdf", gopd.ParseOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, page := range result.Pages {
		for _, text := range page.Texts {
			fmt.Println(text.Unicode)
		}
	}
	// Output:
	// GoPD synthetic fixture
	// Alpha beta 123
	// Page two: shared resources
	// Reusable form
}

func ExampleParseFile_full() {
	result, err := gopd.ParseFile("testdata/synthetic.pdf", gopd.ParseOptions{
		Content: gopd.ContentText,
		Detail:  gopd.DetailFull,
	})
	if result != nil {
		defer result.Close() // Also release a partial result returned with an error.
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Pages:", len(result.Pages))
	fmt.Println("Text runs:", len(result.Details().Texts))
	// Output:
	// Pages: 2
	// Text runs: 4
}

func ExampleParseReader() {
	f, err := os.Open("testdata/synthetic.pdf")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close() // The caller owns the reader.
	info, err := f.Stat()
	if err != nil {
		fmt.Println(err)
		return
	}
	result, err := gopd.ParseReader(f, info.Size(), gopd.ParseOptions{
		Content: gopd.ContentText,
		Detail:  gopd.DetailLayout,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	text := result.Pages[0].Texts[0]
	fmt.Println(text.Unicode)
	fmt.Println("Position and style:", text.Position != nil && text.Style != nil)
	// Output:
	// GoPD synthetic fixture
	// Position and style: true
}
