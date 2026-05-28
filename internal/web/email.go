package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ephemeral-link/internal/redisstore"
)

func (a *App) sendUploadRequestEmail(ctx context.Context, lang, to, link, requesterName, message string) error {
	lang = a.i18n.Normalize(lang)
	subject := a.i18n.T(lang, "upload_request_email_subject")
	body := fmt.Sprintf("%s\n\n%s\n\n%s", fmt.Sprintf(a.i18n.T(lang, "upload_request_email_body"), requesterName, link), strings.TrimSpace(message), a.i18n.T(lang, "link_security_note"))
	return a.sendEmail(ctx, to, subject, body)
}

func (a *App) sendUploadNotificationEmail(ctx context.Context, lang, to, link string) error {
	lang = a.i18n.Normalize(lang)
	subject := a.i18n.T(lang, "upload_notification_email_subject")
	body := fmt.Sprintf(a.i18n.T(lang, "upload_notification_email_body"), link)
	return a.sendEmail(ctx, to, subject, body)
}

func (a *App) sendCreatedLinkEmail(ctx context.Context, lang, to, link, creatorName, kind string) error {
	lang = a.i18n.Normalize(lang)
	localizedKind := a.i18n.T(lang, "email_kind_"+kind)
	subject := fmt.Sprintf(a.i18n.T(lang, "created_link_email_subject_named"), strings.TrimSpace(creatorName))
	plain := fmt.Sprintf(a.i18n.T(lang, "created_link_email_body"), creatorName, localizedKind, link) + "\n\n" + a.i18n.T(lang, "link_security_note")
	htmlBody := createdLinkHTML(createdLinkEmailView{
		CreatorName: creatorName,
		Kind: localizedKind,
		Link: link,
		SecurityNote: a.i18n.T(lang, "link_security_note"),
		Heading: a.i18n.T(lang, "created_link_email_heading"),
		Intro: fmt.Sprintf(a.i18n.T(lang, "created_link_email_intro"), creatorName, localizedKind),
		Description: a.i18n.T(lang, "created_link_email_description"),
		Button: a.i18n.T(lang, "created_link_email_button"),
		Fallback: a.i18n.T(lang, "created_link_email_fallback"),
	})
	return a.sendEmailContent(ctx, to, subject, emailContent{Plain: plain, HTML: htmlBody})
}

type emailContent struct { Plain string; HTML string }

func (a *App) sendEmail(ctx context.Context, to, subject, body string) error {
	return a.sendEmailContent(ctx, to, subject, emailContent{Plain: body})
}

func (a *App) sendEmailContent(ctx context.Context, to, subject string, content emailContent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	cfg, err := a.store.GetIntegrationConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.GraphEnabled {
		return sendGraph(ctx, http.DefaultClient, cfg, to, subject, content)
	}
	if !cfg.SMTPEnabled {
		return errors.New("email delivery is not enabled")
	}
	return sendSMTP(cfg, to, subject, content)
}

func sendSMTP(cfg redisstore.IntegrationConfig, to, subject string, content emailContent) error {
	from := strings.TrimSpace(cfg.SMTPFrom)
	if from == "" {
		from = strings.TrimSpace(cfg.SMTPUsername)
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("invalid recipient address")
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return fmt.Errorf("invalid from address")
	}
	if cfg.SMTPHost == "" || cfg.SMTPPort == "" {
		return errors.New("SMTP host and port are required")
	}

	addr := net.JoinHostPort(cfg.SMTPHost, cfg.SMTPPort)
	var auth smtp.Auth
	if cfg.SMTPUsername != "" || cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	}

	msg := buildMIMEMessage(from, to, subject, content)
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

type graphHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

func sendGraph(ctx context.Context, client graphHTTPClient, cfg redisstore.IntegrationConfig, to, subject string, content emailContent) error {
	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("invalid recipient address")
	}
	if cfg.GraphTenantID == "" || cfg.GraphClientID == "" || cfg.GraphClientSecret == "" || cfg.GraphSender == "" {
		return errors.New("Graph tenant, client ID, client secret, and sender are required")
	}
	token, err := graphToken(ctx, client, cfg)
	if err != nil {
		return err
	}
	return graphSendMail(ctx, client, cfg.GraphSender, token, to, subject, content)
}

func graphToken(ctx context.Context, client graphHTTPClient, cfg redisstore.IntegrationConfig) (string, error) {
	form := url.Values{}
	form.Set("client_id", cfg.GraphClientID)
	form.Set("client_secret", cfg.GraphClientSecret)
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "https://graph.microsoft.com/.default")
	endpoint := "https://login.microsoftonline.com/" + url.PathEscape(cfg.GraphTenantID) + "/oauth2/v2.0/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("Graph token request failed with status %d: %s", res.StatusCode, safeGraphError(data))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.AccessToken == "" {
		return "", errors.New("Graph token response did not include an access token")
	}
	return parsed.AccessToken, nil
}

func graphSendMail(ctx context.Context, client graphHTTPClient, sender, token, to, subject string, content emailContent) error {
	data := []byte(base64.StdEncoding.EncodeToString(buildMIMEMessage(sender, to, subject, content)))
	endpoint := "https://graph.microsoft.com/v1.0/users/" + url.PathEscape(sender) + "/sendMail"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	responseData, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Graph sendMail failed with status %d: %s", res.StatusCode, safeGraphError(responseData))
	}
	return nil
}

func buildMIMEMessage(from, to, subject string, content emailContent) []byte {
	if strings.TrimSpace(content.Plain) == "" {
		content.Plain = "This message contains a secure Ephemeral Link notification."
	}
	boundary := "ephemeral-link-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var msg bytes.Buffer
	msg.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	msg.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	msg.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(content.HTML) == "" {
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		msg.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		msg.WriteString(wrapBase64(content.Plain))
		return msg.Bytes()
	}
	msg.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	msg.WriteString("--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	msg.WriteString(wrapBase64(content.Plain))
	msg.WriteString("\r\n--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	msg.WriteString(wrapBase64(content.HTML))
	msg.WriteString("\r\n--" + boundary + "--\r\n")
	return msg.Bytes()
}

type createdLinkEmailView struct {
	CreatorName string
	Kind string
	Link string
	SecurityNote string
	Heading string
	Intro string
	Description string
	Button string
	Fallback string
}

func createdLinkHTML(view createdLinkEmailView) string {
	linkEscaped := html.EscapeString(view.Link)
	note := html.EscapeString(view.SecurityNote)
	heading := html.EscapeString(view.Heading)
	intro := html.EscapeString(view.Intro)
	description := html.EscapeString(view.Description)
	button := html.EscapeString(view.Button)
	fallback := html.EscapeString(view.Fallback)
	return `<!doctype html>
<html><body style="margin:0;background:#f6f8fb;font-family:Arial,sans-serif;color:#172033;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f6f8fb;padding:32px 16px;"><tr><td align="center">
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:560px;background:#ffffff;border:1px solid #dfe7ef;border-radius:18px;overflow:hidden;box-shadow:0 20px 50px rgba(15,23,42,.08);">
      <tr><td style="background:linear-gradient(135deg,#f97316,#ffb020);padding:26px 30px;color:#111827;"><div style="font-size:13px;text-transform:uppercase;letter-spacing:.14em;font-weight:700;">Ephemeral Link</div><h1 style="margin:8px 0 0;font-size:26px;line-height:1.2;">` + heading + `</h1></td></tr>
      <tr><td style="padding:30px;"><p style="font-size:16px;line-height:1.6;margin:0 0 18px;">` + intro + `</p><p style="font-size:14px;line-height:1.6;color:#42536a;margin:0 0 24px;">` + description + `</p><p style="text-align:center;margin:28px 0;"><a href="` + linkEscaped + `" style="display:inline-block;background:#f97316;color:#111827;text-decoration:none;font-weight:700;padding:14px 22px;border-radius:12px;">` + button + `</a></p><p style="font-size:13px;line-height:1.6;color:#64748b;margin:0 0 10px;">` + fallback + `</p><p style="font-size:13px;line-height:1.6;word-break:break-all;background:#f8fafc;border:1px solid #e2e8f0;border-radius:10px;padding:12px;margin:0 0 22px;"><a href="` + linkEscaped + `" style="color:#c2410c;">` + linkEscaped + `</a></p><div style="border-top:1px solid #e2e8f0;padding-top:18px;color:#64748b;font-size:13px;line-height:1.6;">` + note + `</div></td></tr>
    </table>
  </td></tr></table>
</body></html>`
}

func wrapBase64(value string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	var out strings.Builder
	for len(encoded) > 76 {
		out.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	out.WriteString(encoded + "\r\n")
	return out.String()
}

func safeGraphError(data []byte) string {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "empty response body"
	}
	if len(text) > 500 {
		return text[:500]
	}
	return text
}

func sanitizeHeader(v string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(v) }
