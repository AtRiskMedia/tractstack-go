// Package templates provides email template layout
package templates

import (
	"bytes"
	"html/template"
	"log"
)

type EmailLayoutProps struct {
	Preheader      string
	Content        string
	FooterText     string
	CompanyAddress string
	UnsubscribeURL string
	PoweredByText  string
	PoweredByURL   string
}

// Internal template data structure with safe HTML typing
type emailTemplateData struct {
	Preheader      string
	Content        template.HTML // Mark as safe HTML to prevent escaping
	FooterText     string
	CompanyAddress string
	UnsubscribeURL string
	PoweredByText  string
	PoweredByURL   string
}

// emailLayoutTemplate is the compiled template for email layout
var emailLayoutTemplate = template.Must(template.New("emailLayout").Parse(`
<!doctype html>
<html lang="en">
  <head>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
    <title>Email from TractStack</title>
    <style media="all" type="text/css">
      @media all {
        .btn-primary table td:hover { background-color: #ec0867 !important; }
        .btn-primary a:hover { background-color: #ec0867 !important; border-color: #ec0867 !important; }
      }
      @media only screen and (max-width: 640px) {
        .main p, .main td, .main span { font-size: 16px !important; }
        .wrapper { padding: 8px !important; }
        .content { padding: 0 !important; }
        .container { padding: 0 !important; padding-top: 8px !important; width: 100% !important; }
        .main { border-left-width: 0 !important; border-radius: 0 !important; border-right-width: 0 !important; }
      }
      @media only screen and (max-width: 480px) {
        .main p, .main td, .main span { font-size: 16px !important; }
      }
    </style>
  </head>
  <body style="font-family: Helvetica, sans-serif; -webkit-font-smoothing: antialiased; font-size: 16px; line-height: 1.3; -ms-text-size-adjust: 100%; -webkit-text-size-adjust: 100%; background-color: #f4f5f6; margin: 0; padding: 0;">
    <span class="preheader" style="color: transparent; display: none; height: 0; max-height: 0; max-width: 0; opacity: 0; overflow: hidden; mso-hide: all; visibility: hidden; width: 0;">{{.Preheader}}</span>
    <table role="presentation" border="0" cellpadding="0" cellspacing="0" class="body" style="border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; background-color: #f4f5f6; width: 100%;" width="100%" bgcolor="#f4f5f6">
      <tr>
        <td style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top;" valign="top">&nbsp;</td>
        <td class="container" style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top; max-width: 600px; padding: 0; padding-top: 24px; width: 600px; margin: 0 auto;" width="600" valign="top">
          <div class="content" style="box-sizing: border-box; display: block; margin: 0 auto; max-width: 600px; padding: 0;">
            <!-- START CENTERED WHITE CONTAINER -->
            <table role="presentation" border="0" cellpadding="0" cellspacing="0" class="main" style="border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; background: #ffffff; border: 1px solid #eaebed; border-radius: 16px; width: 100%;" width="100%">
              <!-- START MAIN CONTENT AREA -->
              <tr>
                <td class="wrapper" style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top; box-sizing: border-box; padding: 24px;" valign="top">
                  {{.Content}}
                </td>
              </tr>
              <!-- END MAIN CONTENT AREA -->
            </table>
            <!-- START FOOTER -->
            <div class="footer" style="clear: both; padding-top: 24px; text-align: center; width: 100%;">
              <table role="presentation" border="0" cellpadding="0" cellspacing="0" style="border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; width: 100%;" width="100%">
                <tr>
                  <td class="content-block" style="font-family: Helvetica, sans-serif; vertical-align: top; color: #9a9ea6; font-size: 16px; text-align: center;" valign="top" align="center">
                    <span class="apple-link" style="color: #9a9ea6; font-size: 16px; text-align: center;">{{.FooterText}}</span>
                    <br>{{.CompanyAddress}}
                    <br><a href="{{.UnsubscribeURL}}" style="text-decoration: underline; color: #9a9ea6; font-size: 16px; text-align: center;">Unsubscribe</a>.
                  </td>
                </tr>
                <tr>
                  <td class="content-block powered-by" style="font-family: Helvetica, sans-serif; vertical-align: top; color: #9a9ea6; font-size: 16px; text-align: center;" valign="top" align="center">
                    Powered by <a href="{{.PoweredByURL}}" style="color: #9a9ea6; font-size: 16px; text-align: center; text-decoration: none;">{{.PoweredByText}}</a>
                  </td>
                </tr>
              </table>
            </div>
            <!-- END FOOTER -->
          </div>
        </td>
        <td style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top;" valign="top">&nbsp;</td>
      </tr>
    </table>
  </body>
</html>`))

func GetEmailLayout(props EmailLayoutProps) string {
	// Set defaults exactly as before
	preheader := props.Preheader
	if preheader == "" {
		preheader = "no-code community engine and website maker"
	}

	footerText := props.FooterText
	if footerText == "" {
		footerText = "no-code community engine and website maker"
	}

	companyAddress := props.CompanyAddress
	if companyAddress == "" {
		companyAddress = "Proudly Canadian"
	}

	unsubscribeURL := props.UnsubscribeURL
	if unsubscribeURL == "" {
		unsubscribeURL = "http://example.com/unsubscribe"
	}

	poweredByText := props.PoweredByText
	if poweredByText == "" {
		poweredByText = "TractStack"
	}

	poweredByURL := props.PoweredByURL
	if poweredByURL == "" {
		poweredByURL = "https://tractstack.com"
	}

	// Create template data with defaults applied and safe HTML for Content
	templateData := emailTemplateData{
		Preheader:      preheader,
		Content:        template.HTML(props.Content), // Convert to safe HTML type
		FooterText:     footerText,
		CompanyAddress: companyAddress,
		UnsubscribeURL: unsubscribeURL,
		PoweredByText:  poweredByText,
		PoweredByURL:   poweredByURL,
	}

	// Execute template
	var buf bytes.Buffer
	if err := emailLayoutTemplate.Execute(&buf, templateData); err != nil {
		log.Printf("Error executing email layout template: %v", err)
		return "<html><body>Template execution error</body></html>"
	}

	return buf.String()
}
