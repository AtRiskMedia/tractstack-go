// Package templates provides email template components
package templates

import "fmt"

type ButtonProps struct {
	Text            string
	URL             string
	BackgroundColor string
	TextColor       string
}

func GetButton(props ButtonProps) string {
	backgroundColor := props.BackgroundColor
	if backgroundColor == "" {
		backgroundColor = "#0867ec"
	}

	textColor := props.TextColor
	if textColor == "" {
		textColor = "#ffffff"
	}

	return fmt.Sprintf(`
    <table role="presentation" border="0" cellpadding="0" cellspacing="0" class="btn btn-primary" style="border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; box-sizing: border-box; width: 100%%; min-width: 100%%;" width="100%%">
      <tbody>
        <tr>
          <td align="left" style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top; padding-bottom: 16px;" valign="top">
            <table role="presentation" border="0" cellpadding="0" cellspacing="0" style="border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; width: auto;">
              <tbody>
                <tr>
                  <td style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top; border-radius: 4px; text-align: center; background-color: %s;" valign="top" align="center" bgcolor="%s">
                    <a href="%s" target="_blank" style="border: solid 2px %s; border-radius: 4px; box-sizing: border-box; cursor: pointer; display: inline-block; font-size: 16px; font-weight: bold; margin: 0; padding: 12px 24px; text-decoration: none; text-transform: capitalize; background-color: %s; border-color: %s; color: %s;">%s</a>
                  </td>
                </tr>
              </tbody>
            </table>
          </td>
        </tr>
      </tbody>
    </table>`, backgroundColor, backgroundColor, props.URL, backgroundColor, backgroundColor, backgroundColor, textColor, props.Text)
}

func GetParagraph(text string) string {
	return fmt.Sprintf(`<p style="font-family: Helvetica, sans-serif; font-size: 16px; font-weight: normal; margin: 0; margin-bottom: 16px;">%s</p>`, text)
}
