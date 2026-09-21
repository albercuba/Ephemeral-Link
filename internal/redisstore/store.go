package redisstore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Item struct {
	ID                   string
	Type                 string
	Status               string
	CreatedAt            int64
	ExpiresAt            int64
	CreatedBy            string
	Direction            string
	HasPassphrase        bool
	PassphraseHash       string
	WrappedKeyNonce      string
	WrappedKeyCiphertext string
	PayloadNonce         string
	PayloadCiphertext    string
	OriginalFilename     string
	SanitizedFilename    string
	FileSize             int64
	MimeType             string
	StorageObjectPath    string
}

type AuditEvent struct {
	ID        string
	CreatedAt int64
	Actor     string
	IP        string
	Event     string
	Target    string
	Result    string
	Details   string
}

type Store struct{ rdb *redis.Client }

func New(url string) (*Store, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &Store{rdb: redis.NewClient(opt)}, nil
}
func (s *Store) Ping(ctx context.Context) error { return s.rdb.Ping(ctx).Err() }
func (s *Store) Close() error                   { return s.rdb.Close() }
func key(id string) string                      { return "el:item:" + id }
func uploadRequestKey(id string) string         { return "el:upload_request:" + id }
func auditKey(id string) string                 { return "el:audit:" + id }

const auditIndexKey = "el:audit:index"

func (s *Store) Create(ctx context.Context, item Item, ttl time.Duration) error {
	m := map[string]any{
		"id": item.ID, "type": item.Type, "status": "available", "created_at": item.CreatedAt, "expires_at": item.ExpiresAt, "created_by": item.CreatedBy, "direction": item.Direction,
		"has_passphrase": boolString(item.HasPassphrase), "passphrase_hash": item.PassphraseHash,
		"wrapped_key_nonce": item.WrappedKeyNonce, "wrapped_key_ciphertext": item.WrappedKeyCiphertext,
		"payload_nonce": item.PayloadNonce, "payload_ciphertext": item.PayloadCiphertext,
		"original_filename": item.OriginalFilename, "sanitized_filename": item.SanitizedFilename, "file_size": item.FileSize,
		"mime_type": item.MimeType, "storage_object_path": item.StorageObjectPath,
	}
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, key(item.ID), m)
	pipe.Expire(ctx, key(item.ID), ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) Get(ctx context.Context, id string) (Item, error) {
	return parse(s.rdb.HGetAll(ctx, key(id)).Result())
}

var ErrGone = errors.New("item unavailable")

var claimScript = redis.NewScript(`
local k = KEYS[1]
if redis.call('EXISTS', k) == 0 then return nil end
local st = redis.call('HGET', k, 'status')
if st ~= 'available' then return nil end
local item = redis.call('HGETALL', k)
redis.call('HSET', k, 'status', 'consumed', 'consumed_at', ARGV[1])
redis.call('HDEL', k, 'wrapped_key_nonce', 'wrapped_key_ciphertext', 'payload_nonce', 'payload_ciphertext', 'storage_object_path')
redis.call('EXPIRE', k, ARGV[2])
return item
`)

func (s *Store) Claim(ctx context.Context, id string) (Item, error) {
	res, err := claimScript.Run(ctx, s.rdb, []string{key(id)}, time.Now().Unix(), 3600).Result()
	if err == redis.Nil || res == nil {
		return Item{}, ErrGone
	}
	if err != nil {
		return Item{}, err
	}
	arr, ok := res.([]interface{})
	if !ok {
		return Item{}, fmt.Errorf("unexpected redis claim response")
	}
	m := map[string]string{}
	for i := 0; i+1 < len(arr); i += 2 {
		m[fmt.Sprint(arr[i])] = fmt.Sprint(arr[i+1])
	}
	return parseMap(m, nil)
}

func parse(m map[string]string, err error) (Item, error) { return parseMap(m, err) }
func parseMap(m map[string]string, err error) (Item, error) {
	if err != nil {
		return Item{}, err
	}
	if len(m) == 0 {
		return Item{}, ErrGone
	}
	direction := m["direction"]
	if direction == "" {
		direction = "send"
	}
	return Item{ID: m["id"], Type: m["type"], Status: m["status"], CreatedAt: i64(m["created_at"]), ExpiresAt: i64(m["expires_at"]), CreatedBy: m["created_by"], Direction: direction, HasPassphrase: m["has_passphrase"] == "true", PassphraseHash: m["passphrase_hash"], WrappedKeyNonce: m["wrapped_key_nonce"], WrappedKeyCiphertext: m["wrapped_key_ciphertext"], PayloadNonce: m["payload_nonce"], PayloadCiphertext: m["payload_ciphertext"], OriginalFilename: m["original_filename"], SanitizedFilename: m["sanitized_filename"], FileSize: i64(m["file_size"]), MimeType: m["mime_type"], StorageObjectPath: m["storage_object_path"]}, nil
}
func i64(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

type User struct {
	Username     string
	PasswordHash string
	Role         string
	FirstName    string
	LastName     string
	Email        string
	CreatedAt    int64
}

type UploadRequest struct {
	ID             string
	Status         string
	CreatedAt      int64
	ExpiresAt      int64
	RequestedBy    string
	RequesterEmail string
	RecipientEmail string
	Message        string
	UploadedItemID string
	UploadedAt     int64
}

type IntegrationConfig struct {
	MicrosoftEnabled    bool
	MicrosoftTenantID   string
	MicrosoftClientID   string
	MicrosoftAudience   string
	MicrosoftAuthority  string
	EntraAdminGroupName string
	EntraAdminGroupID   string
	EntraUserGroupName  string
	EntraUserGroupID    string
	ADEnabled           bool
	ADHost              string
	ADBaseDN            string
	ADBindDN            string
	SMTPEnabled         bool
	SMTPHost            string
	SMTPPort            string
	SMTPUsername        string
	SMTPPassword        string
	SMTPFrom            string
	GraphEnabled        bool
	GraphTenantID       string
	GraphClientID       string
	GraphClientSecret   string
	GraphSender         string
}

func userKey(username string) string { return "el:user:" + username }
func sessionKey(token string) string { return "el:session:" + token }

func (s *Store) EnsureUser(ctx context.Context, user User) error {
	exists, err := s.rdb.Exists(ctx, userKey(user.Username)).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	return s.rdb.HSet(ctx, userKey(user.Username), userMap(user)).Err()
}
func (s *Store) SaveUser(ctx context.Context, user User) error {
	return s.rdb.HSet(ctx, userKey(user.Username), userMap(user)).Err()
}
func (s *Store) DeleteUser(ctx context.Context, username string) error {
	return s.rdb.Del(ctx, userKey(username)).Err()
}
func (s *Store) GetUser(ctx context.Context, username string) (User, error) {
	m, err := s.rdb.HGetAll(ctx, userKey(username)).Result()
	if err != nil {
		return User{}, err
	}
	if len(m) == 0 {
		return User{}, ErrGone
	}
	return userFromMap(m), nil
}
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	keys, err := s.rdb.Keys(ctx, "el:user:*").Result()
	if err != nil {
		return nil, err
	}
	out := []User{}
	for _, k := range keys {
		m, err := s.rdb.HGetAll(ctx, k).Result()
		if err == nil && len(m) > 0 {
			out = append(out, userFromMap(m))
		}
	}
	return out, nil
}
func (s *Store) HasAdmin(ctx context.Context) (bool, error) {
	users, err := s.ListUsers(ctx)
	if err != nil {
		return false, err
	}
	for _, user := range users {
		if user.Role == "administrator" {
			return true, nil
		}
	}
	return false, nil
}
func (s *Store) CreateInitialAdmin(ctx context.Context, user User) error {
	locked, err := s.rdb.SetNX(ctx, "el:setup:admin_lock", "1", time.Minute).Result()
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("setup is already in progress")
	}
	defer s.rdb.Del(ctx, "el:setup:admin_lock")
	exists, err := s.HasAdmin(ctx)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("administrator already exists")
	}
	user.Role = "administrator"
	return s.SaveUser(ctx, user)
}
func (s *Store) CreateSession(ctx context.Context, token, username string, ttl time.Duration) error {
	return s.rdb.Set(ctx, sessionKey(token), username, ttl).Err()
}
func (s *Store) GetSession(ctx context.Context, token string) (string, error) {
	username, err := s.rdb.Get(ctx, sessionKey(token)).Result()
	if err == redis.Nil {
		return "", ErrGone
	}
	return username, err
}
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, sessionKey(token)).Err()
}
func (s *Store) ListAvailableItems(ctx context.Context) ([]Item, error) {
	keys, err := s.rdb.Keys(ctx, "el:item:*").Result()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := []Item{}
	for _, k := range keys {
		item, err := parse(s.rdb.HGetAll(ctx, k).Result())
		if err == nil && item.Status == "available" && item.ExpiresAt > now {
			out = append(out, item)
		}
	}
	return out, nil
}
func (s *Store) ActiveStorageObjectPaths(ctx context.Context) (map[string]struct{}, error) {
	keys, err := s.rdb.Keys(ctx, "el:item:*").Result()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	active := map[string]struct{}{}
	for _, k := range keys {
		item, err := parse(s.rdb.HGetAll(ctx, k).Result())
		if err == nil && item.Status == "available" && item.ExpiresAt > now && item.StorageObjectPath != "" {
			active[item.StorageObjectPath] = struct{}{}
		}
	}
	return active, nil
}
func (s *Store) BurnItem(ctx context.Context, id string) error {
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, key(id), map[string]any{"status": "consumed", "consumed_at": time.Now().Unix(), "wrapped_key_nonce": "", "wrapped_key_ciphertext": "", "payload_nonce": "", "payload_ciphertext": "", "storage_object_path": ""})
	pipe.Expire(ctx, key(id), time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}
func (s *Store) BurnUploadRequest(ctx context.Context, id string) error {
	return s.rdb.HSet(ctx, uploadRequestKey(id), "status", "consumed", "consumed_at", time.Now().Unix()).Err()
}
func (s *Store) CreateUploadRequest(ctx context.Context, req UploadRequest, ttl time.Duration) error {
	m := map[string]any{"id": req.ID, "status": "available", "created_at": req.CreatedAt, "expires_at": req.ExpiresAt, "requested_by": req.RequestedBy, "requester_email": req.RequesterEmail, "recipient_email": req.RecipientEmail, "message": req.Message, "uploaded_item_id": req.UploadedItemID, "uploaded_at": req.UploadedAt}
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, uploadRequestKey(req.ID), m)
	pipe.Expire(ctx, uploadRequestKey(req.ID), ttl)
	_, err := pipe.Exec(ctx)
	return err
}
func (s *Store) GetUploadRequest(ctx context.Context, id string) (UploadRequest, error) {
	m, err := s.rdb.HGetAll(ctx, uploadRequestKey(id)).Result()
	return uploadRequestFromMap(m, err)
}
func (s *Store) ListAvailableUploadRequests(ctx context.Context) ([]UploadRequest, error) {
	keys, err := s.rdb.Keys(ctx, "el:upload_request:*").Result()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := []UploadRequest{}
	for _, k := range keys {
		req, err := uploadRequestFromMap(s.rdb.HGetAll(ctx, k).Result())
		if err == nil && req.Status == "available" && req.ExpiresAt > now {
			out = append(out, req)
		}
	}
	return out, nil
}

var claimUploadRequestScript = redis.NewScript(`
local k = KEYS[1]
if redis.call('EXISTS', k) == 0 then return nil end
local st = redis.call('HGET', k, 'status')
if st ~= 'available' then return nil end
redis.call('HSET', k, 'status', 'uploading')
return redis.call('HGETALL', k)
`)

func (s *Store) ClaimUploadRequest(ctx context.Context, id string) (UploadRequest, error) {
	res, err := claimUploadRequestScript.Run(ctx, s.rdb, []string{uploadRequestKey(id)}).Result()
	if err == redis.Nil || res == nil {
		return UploadRequest{}, ErrGone
	}
	if err != nil {
		return UploadRequest{}, err
	}
	arr, ok := res.([]interface{})
	if !ok {
		return UploadRequest{}, fmt.Errorf("unexpected redis upload request claim response")
	}
	m := map[string]string{}
	for i := 0; i+1 < len(arr); i += 2 {
		m[fmt.Sprint(arr[i])] = fmt.Sprint(arr[i+1])
	}
	return uploadRequestFromMap(m, nil)
}

func (s *Store) MarkUploadRequestUploaded(ctx context.Context, id, itemID string) error {
	return s.rdb.HSet(ctx, uploadRequestKey(id), "status", "uploaded", "uploaded_item_id", itemID, "uploaded_at", time.Now().Unix()).Err()
}
func (s *Store) ReleaseUploadRequest(ctx context.Context, id string) error {
	return s.rdb.HSet(ctx, uploadRequestKey(id), "status", "available").Err()
}
func (s *Store) SaveIntegrationConfig(ctx context.Context, cfg IntegrationConfig) error {
	return s.rdb.HSet(ctx, "el:integration", map[string]any{"microsoft_enabled": boolString(cfg.MicrosoftEnabled), "microsoft_tenant_id": cfg.MicrosoftTenantID, "microsoft_client_id": cfg.MicrosoftClientID, "microsoft_audience": cfg.MicrosoftAudience, "microsoft_authority": cfg.MicrosoftAuthority, "entra_admin_group_name": cfg.EntraAdminGroupName, "entra_admin_group_id": cfg.EntraAdminGroupID, "entra_user_group_name": cfg.EntraUserGroupName, "entra_user_group_id": cfg.EntraUserGroupID, "ad_enabled": boolString(cfg.ADEnabled), "ad_host": cfg.ADHost, "ad_base_dn": cfg.ADBaseDN, "ad_bind_dn": cfg.ADBindDN, "smtp_enabled": boolString(cfg.SMTPEnabled), "smtp_host": cfg.SMTPHost, "smtp_port": cfg.SMTPPort, "smtp_username": cfg.SMTPUsername, "smtp_password": cfg.SMTPPassword, "smtp_from": cfg.SMTPFrom, "graph_enabled": boolString(cfg.GraphEnabled), "graph_tenant_id": cfg.GraphTenantID, "graph_client_id": cfg.GraphClientID, "graph_client_secret": cfg.GraphClientSecret, "graph_sender": cfg.GraphSender}).Err()
}
func (s *Store) GetIntegrationConfig(ctx context.Context) (IntegrationConfig, error) {
	m, err := s.rdb.HGetAll(ctx, "el:integration").Result()
	if err != nil {
		return IntegrationConfig{}, err
	}
	return IntegrationConfig{MicrosoftEnabled: m["microsoft_enabled"] == "true", MicrosoftTenantID: m["microsoft_tenant_id"], MicrosoftClientID: m["microsoft_client_id"], MicrosoftAudience: m["microsoft_audience"], MicrosoftAuthority: m["microsoft_authority"], EntraAdminGroupName: m["entra_admin_group_name"], EntraAdminGroupID: m["entra_admin_group_id"], EntraUserGroupName: m["entra_user_group_name"], EntraUserGroupID: m["entra_user_group_id"], ADEnabled: m["ad_enabled"] == "true", ADHost: m["ad_host"], ADBaseDN: m["ad_base_dn"], ADBindDN: m["ad_bind_dn"], SMTPEnabled: m["smtp_enabled"] == "true", SMTPHost: m["smtp_host"], SMTPPort: m["smtp_port"], SMTPUsername: m["smtp_username"], SMTPPassword: m["smtp_password"], SMTPFrom: m["smtp_from"], GraphEnabled: m["graph_enabled"] == "true", GraphTenantID: m["graph_tenant_id"], GraphClientID: m["graph_client_id"], GraphClientSecret: m["graph_client_secret"], GraphSender: m["graph_sender"]}, nil
}

func (s *Store) AddAuditEvent(ctx context.Context, event AuditEvent) error {
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().Unix()
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	m := map[string]any{"id": event.ID, "created_at": event.CreatedAt, "actor": event.Actor, "ip": event.IP, "event": event.Event, "target": event.Target, "result": event.Result, "details": event.Details}
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, auditKey(event.ID), m)
	pipe.ZAdd(ctx, auditIndexKey, redis.Z{Score: float64(event.CreatedAt), Member: event.ID})
	pipe.Expire(ctx, auditKey(event.ID), 90*24*time.Hour)
	pipe.ZRemRangeByRank(ctx, auditIndexKey, 0, -501)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context, limit int64) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	ids, err := s.rdb.ZRevRange(ctx, auditIndexKey, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}
	out := []AuditEvent{}
	for _, id := range ids {
		m, err := s.rdb.HGetAll(ctx, auditKey(id)).Result()
		if err == nil && len(m) > 0 {
			out = append(out, auditEventFromMap(m))
		}
	}
	return out, nil
}

func userMap(user User) map[string]any {
	return map[string]any{"username": user.Username, "password_hash": user.PasswordHash, "role": user.Role, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email, "created_at": user.CreatedAt}
}
func userFromMap(m map[string]string) User {
	return User{Username: m["username"], PasswordHash: m["password_hash"], Role: m["role"], FirstName: m["first_name"], LastName: m["last_name"], Email: m["email"], CreatedAt: i64(m["created_at"])}
}
func auditEventFromMap(m map[string]string) AuditEvent {
	return AuditEvent{ID: m["id"], CreatedAt: i64(m["created_at"]), Actor: m["actor"], IP: m["ip"], Event: m["event"], Target: m["target"], Result: m["result"], Details: m["details"]}
}
func uploadRequestFromMap(m map[string]string, err error) (UploadRequest, error) {
	if err != nil {
		return UploadRequest{}, err
	}
	if len(m) == 0 {
		return UploadRequest{}, ErrGone
	}
	return UploadRequest{ID: m["id"], Status: m["status"], CreatedAt: i64(m["created_at"]), ExpiresAt: i64(m["expires_at"]), RequestedBy: m["requested_by"], RequesterEmail: m["requester_email"], RecipientEmail: m["recipient_email"], Message: m["message"], UploadedItemID: m["uploaded_item_id"], UploadedAt: i64(m["uploaded_at"])}, nil
}
