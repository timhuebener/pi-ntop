package components

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
)

func StatCard(label, value, detail string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := fmt.Fprintf(
			w,
			"<article class=\"stat-card\"><p class=\"stat-label\">%s</p><p class=\"stat-value\">%s</p><p class=\"stat-detail\">%s</p></article>",
			templ.EscapeString(label),
			templ.EscapeString(value),
			templ.EscapeString(detail),
		)
		return err
	})
}
