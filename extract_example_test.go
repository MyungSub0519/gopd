package gopd_test

import (
	"fmt"

	"github.com/MyungSub0519/gopd"
)

func ExampleExtract() {
	result, err := gopd.Extract("testdata/synthetic.pdf", gopd.ExtractOptions{})
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
