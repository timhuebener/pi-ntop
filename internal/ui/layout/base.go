package layout

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

func AppShell(title string, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(title)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</title><link rel=\"stylesheet\" href=\"/assets/css/output.css\"></head><body>"); err != nil {
			return err
		}
		if err := body.Render(ctx, w); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</body></html>")
		return err
	})
}
