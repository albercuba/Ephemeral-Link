package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"

	"ephemeral-link/internal/redisstore"
)

func (a *App) sendUploadRequestEmail(ctx context.Context, to, link, requesterName, message string) error {
	subject := a.i18n.T(a.cfg.DefaultLanguage, "upload_request_email_subject")
	body := fmt.Sprintf("%s\n\n%s\n\n%s", fmt.Sprintf(a.i18n.T(a.cfg.DefaultLanguage, "upload_request_email_body"), requesterName, link), strings.TrimSpace(message), a.i18n.T(a.cfg.DefaultLanguage, "link_security_note"))
	return a.sendEmail(ctx, to, subject, body)
}

func (a *App) sendUploadNotificationEmail(ctx context.Context, to, link string) error {
	subject := a.i18n.T(a.cfg.DefaultLanguage, "upload_notification_email_subject")
	body := fmt.Sprintf(a.i18n.T(a.cfg.DefaultLanguage, "upload_notification_email_body"), link)
	return a.sendEmail(ctx, to, subject, body)
}

func (a *App) sendCreatedLinkEmail(ctx context.Context, to, link, creatorName, kind string) error {
	subject := a.i18n.T(a.cfg.DefaultLanguage, "created_link_email_subject")
	body := fmt.Sprintf(a.i18n.T(a.cfg.DefaultLanguage, "created_link_email_body"), creatorName, kind, link) + "\n\n" + a.i18n.T(a.cfg.DefaultLanguage, "link_security_note")
	return a.sendEmail(ctx, to, subject, body)
}

func (a *App) sendEmail(ctx context.Context, to, subject, body string) error {
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
		return sendGraph(ctx, http.DefaultClient, cfg, to, subject, body)
	}
	if !cfg.SMTPEnabled {
		return errors.New("email delivery is not enabled")
	}
	return sendSMTP(cfg, to, subject, body)
}

func sendSMTP(cfg redisstore.IntegrationConfig, to, subject, body string) error {
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

	var msg bytes.Buffer
	msg.WriteString("From: " + from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	return smtp.SendMail(addr, auth, from, []string{to}, msg.Bytes())
}

type graphHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

func sendGraph(ctx context.Context, client graphHTTPClient, cfg redisstore.IntegrationConfig, to, subject, body string) error {
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
	return graphSendMail(ctx, client, cfg.GraphSender, token, to, subject, body)
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
		return "", fmt.Errorf("Graph token request failed with status %d", res.StatusCode)
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

func graphSendMail(ctx context.Context, client graphHTTPClient, sender, token, to, subject, body string) error {
	payload := map[string]any{"message": map[string]any{"subject": subject, "body": map[string]string{"contentType": "Text", "content": body}, "toRecipients": []map[string]any{{"emailAddress": map[string]string{"address": to}}}}, "saveToSentItems": false}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := "https://graph.microsoft.com/v1.0/users/" + url.PathEscape(sender) + "/sendMail"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Graph sendMail failed with status %d", res.StatusCode)
	}
	return nil
}

func sanitizeHeader(v string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(v) }
