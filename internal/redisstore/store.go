package redisstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const DefaultWorkspaceID = "default"

type Item struct {
	ID                   string
	WorkspaceID          string
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
	WorkspaceID string
	ID          string
	CreatedAt   int64
	Actor       string
	IP          string
	Event       string
	Target      string
	Result      string
	Details     string
}

type WorkspaceMembership struct {
	Username    string
	WorkspaceID string
	Role        string
}

type WorkspaceInvitation struct {
	ID          string
	WorkspaceID string
	Email       string
	Role        string
	CreatedAt   int64
	ExpiresAt   int64
}

type CreatedReceipt struct {
	Token       string
	Link        string
	ExpiresAt   string
	TTLSeconds  int64
	EmailStatus string
}

type APIKey struct {
	WorkspaceID string
	ID          string
	Name        string
	Hash        string
	Scopes      []string
	CreatedAt   int64
	ExpiresAt   int64
	RevokedAt   int64
}

var ErrInvalidAPIKey = errors.New("invalid api key")

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

func (s *Store) scanKeys(ctx context.Context, pattern string) ([]string, error) {
	var cursor uint64
	keys := []string{}
	for {
		batch, next, err := s.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			return keys, nil
		}
	}
}

func key(id string) string                  { return "el:item:" + id }
func uploadRequestKey(id string) string     { return "el:upload_request:" + id }
func auditKey(id string) string             { return "el:audit:" + id }
func createdReceiptKey(token string) string { return "el:created_receipt:" + token }
func apiKeyKey(id string) string            { return "el:api_key:" + id }
func invitationKey(hash string) string      { return "el:workspace_invitation:" + hash }
func membershipKey(username, workspace string) string {
	return "el:membership:" + username + ":" + workspace
}

const auditIndexKey = "el:audit:index"
const apiKeyPrefix = "elak_"

func (s *Store) Create(ctx context.Context, item Item, ttl time.Duration) error {
	m := map[string]any{
		"id": item.ID, "workspace_id": workspaceID(item.WorkspaceID), "type": item.Type, "status": "available", "created_at": item.CreatedAt, "expires_at": item.ExpiresAt, "created_by": item.CreatedBy, "direction": item.Direction,
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
	return Item{ID: m["id"], WorkspaceID: workspaceID(m["workspace_id"]), Type: m["type"], Status: m["status"], CreatedAt: i64(m["created_at"]), ExpiresAt: i64(m["expires_at"]), CreatedBy: m["created_by"], Direction: direction, HasPassphrase: m["has_passphrase"] == "true", PassphraseHash: m["passphrase_hash"], WrappedKeyNonce: m["wrapped_key_nonce"], WrappedKeyCiphertext: m["wrapped_key_ciphertext"], PayloadNonce: m["payload_nonce"], PayloadCiphertext: m["payload_ciphertext"], OriginalFilename: m["original_filename"], SanitizedFilename: m["sanitized_filename"], FileSize: i64(m["file_size"]), MimeType: m["mime_type"], StorageObjectPath: m["storage_object_path"]}, nil
}
func i64(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
func workspaceID(value string) string {
	if strings.TrimSpace(value) == "" {
		return DefaultWorkspaceID
	}
	return strings.TrimSpace(value)
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

type User struct {
	WorkspaceID  string
	Username     string
	PasswordHash string
	Role         string
	FirstName    string
	LastName     string
	Email        string
	AuthProvider string
	ExternalID   string
	CreatedAt    int64
}

type UploadRequest struct {
	WorkspaceID    string
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

type LegalDocuments struct {
	Privacy string
	Terms   string
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
	ADBindPassword      string
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
	keys, err := s.scanKeys(ctx, "el:user:*")
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

func (s *Store) ListUsersInWorkspace(ctx context.Context, workspace string) ([]User, error) {
	if workspace == "" {
		return s.ListUsers(ctx)
	}
	membershipKeys, err := s.scanKeys(ctx, "el:membership:*:"+workspaceID(workspace))
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(membershipKeys))
	seen := make(map[string]struct{}, len(membershipKeys))
	for _, membershipKey := range membershipKeys {
		membership, err := s.rdb.HGetAll(ctx, membershipKey).Result()
		if err != nil || len(membership) == 0 {
			continue
		}
		username := membership["username"]
		if username == "" {
			continue
		}
		if _, ok := seen[username]; ok {
			continue
		}
		m, err := s.rdb.HGetAll(ctx, userKey(username)).Result()
		if err == nil && len(m) > 0 {
			out = append(out, userFromMap(m))
			seen[username] = struct{}{}
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
	return s.rdb.Del(ctx, sessionKey(token), "el:session_workspace:"+token).Err()
}

func (s *Store) AddWorkspaceMembership(ctx context.Context, username, workspace, role string) error {
	return s.rdb.HSet(ctx, membershipKey(username, workspaceID(workspace)), map[string]any{"username": username, "workspace_id": workspaceID(workspace), "role": role}).Err()
}

func (s *Store) HasWorkspaceMembership(ctx context.Context, username, workspace string) (bool, error) {
	_, err := s.GetWorkspaceMembership(ctx, username, workspace)
	if errors.Is(err, ErrGone) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) GetWorkspaceMembership(ctx context.Context, username, workspace string) (WorkspaceMembership, error) {
	m, err := s.rdb.HGetAll(ctx, membershipKey(username, workspaceID(workspace))).Result()
	if err != nil {
		return WorkspaceMembership{}, err
	}
	if len(m) == 0 {
		return WorkspaceMembership{}, ErrGone
	}
	return WorkspaceMembership{Username: m["username"], WorkspaceID: workspaceID(m["workspace_id"]), Role: m["role"]}, nil
}

func (s *Store) RemoveWorkspaceMembership(ctx context.Context, username, workspace string) error {
	return s.rdb.Del(ctx, membershipKey(username, workspaceID(workspace))).Err()
}

func (s *Store) ListWorkspaceMemberships(ctx context.Context, username string) ([]WorkspaceMembership, error) {
	keys, err := s.scanKeys(ctx, "el:membership:"+username+":*")
	if err != nil {
		return nil, err
	}
	memberships := make([]WorkspaceMembership, 0, len(keys))
	for _, key := range keys {
		m, err := s.rdb.HGetAll(ctx, key).Result()
		if err == nil && len(m) > 0 {
			memberships = append(memberships, WorkspaceMembership{Username: m["username"], WorkspaceID: workspaceID(m["workspace_id"]), Role: m["role"]})
		}
	}
	return memberships, nil
}

func (s *Store) SetSessionWorkspace(ctx context.Context, token, workspace string) error {
	return s.rdb.Set(ctx, "el:session_workspace:"+token, workspaceID(workspace), 8*time.Hour).Err()
}

func (s *Store) GetSessionWorkspace(ctx context.Context, token string) (string, error) {
	value, err := s.rdb.Get(ctx, "el:session_workspace:"+token).Result()
	if err == redis.Nil {
		return "", ErrGone
	}
	return value, err
}

func (s *Store) CreateWorkspaceInvitation(ctx context.Context, workspace, email, role string, expiresAt int64) (WorkspaceInvitation, string, error) {
	value, err := newAPIKeyValue()
	if err != nil {
		return WorkspaceInvitation{}, "", err
	}
	created := time.Now().Unix()
	invitation := WorkspaceInvitation{ID: hashAPIKey(value)[:16], WorkspaceID: workspaceID(workspace), Email: strings.ToLower(strings.TrimSpace(email)), Role: role, CreatedAt: created, ExpiresAt: expiresAt}
	if err := s.rdb.HSet(ctx, invitationKey(hashAPIKey(value)), map[string]any{"id": invitation.ID, "workspace_id": invitation.WorkspaceID, "email": invitation.Email, "role": role, "created_at": created, "expires_at": expiresAt}).Err(); err != nil {
		return WorkspaceInvitation{}, "", err
	}
	return invitation, value, nil
}

func (s *Store) GetWorkspaceInvitation(ctx context.Context, value string) (WorkspaceInvitation, error) {
	m, err := s.rdb.HGetAll(ctx, invitationKey(hashAPIKey(value))).Result()
	if err != nil || len(m) == 0 || (i64(m["expires_at"]) > 0 && i64(m["expires_at"]) <= time.Now().Unix()) {
		return WorkspaceInvitation{}, ErrInvalidAPIKey
	}
	return WorkspaceInvitation{ID: m["id"], WorkspaceID: workspaceID(m["workspace_id"]), Email: m["email"], Role: m["role"], CreatedAt: i64(m["created_at"]), ExpiresAt: i64(m["expires_at"])}, nil
}

func (s *Store) AcceptWorkspaceInvitation(ctx context.Context, value string) (WorkspaceInvitation, error) {
	invitation, err := s.GetWorkspaceInvitation(ctx, value)
	if err != nil {
		return WorkspaceInvitation{}, err
	}
	if err := s.rdb.Del(ctx, invitationKey(hashAPIKey(value))).Err(); err != nil {
		return WorkspaceInvitation{}, err
	}
	return invitation, nil
}

func newAPIKeyValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return apiKeyPrefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func hashAPIKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateAPIKey(ctx context.Context, name string, scopes []string, expiresAt int64) (APIKey, string, error) {
	return s.CreateAPIKeyInWorkspace(ctx, DefaultWorkspaceID, name, scopes, expiresAt)
}

func (s *Store) CreateAPIKeyInWorkspace(ctx context.Context, workspace, name string, scopes []string, expiresAt int64) (APIKey, string, error) {
	value, err := newAPIKeyValue()
	if err != nil {
		return APIKey{}, "", err
	}
	idBytes := make([]byte, 12)
	if _, err := rand.Read(idBytes); err != nil {
		return APIKey{}, "", err
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes)
	created := time.Now().Unix()
	key := APIKey{WorkspaceID: workspaceID(workspace), ID: id, Name: name, Hash: hashAPIKey(value), Scopes: append([]string(nil), scopes...), CreatedAt: created, ExpiresAt: expiresAt}
	if err := s.rdb.HSet(ctx, apiKeyKey(id), map[string]any{"workspace_id": key.WorkspaceID, "name": name, "hash": key.Hash, "scopes": strings.Join(scopes, ","), "created_at": created, "expires_at": expiresAt, "revoked_at": 0}).Err(); err != nil {
		return APIKey{}, "", err
	}
	return key, value, nil
}

func (s *Store) AuthenticateAPIKey(ctx context.Context, value, scope string) (APIKey, error) {
	if !strings.HasPrefix(value, apiKeyPrefix) {
		return APIKey{}, ErrInvalidAPIKey
	}
	hash := hashAPIKey(value)
	keys, err := s.scanKeys(ctx, "el:api_key:*")
	if err != nil {
		return APIKey{}, err
	}
	now := time.Now().Unix()
	for _, redisKey := range keys {
		m, err := s.rdb.HGetAll(ctx, redisKey).Result()
		if err != nil || m["hash"] != hash || m["revoked_at"] != "0" || (i64(m["expires_at"]) > 0 && i64(m["expires_at"]) <= now) {
			continue
		}
		key := apiKeyFromMap(redisKey, m)
		if scope != "" && !containsString(key.Scopes, scope) {
			return APIKey{}, ErrInvalidAPIKey
		}
		return key, nil
	}
	return APIKey{}, ErrInvalidAPIKey
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	return s.ListAPIKeysInWorkspace(ctx, "")
}

func (s *Store) ListAPIKeysInWorkspace(ctx context.Context, workspace string) ([]APIKey, error) {
	keys, err := s.scanKeys(ctx, "el:api_key:*")
	if err != nil {
		return nil, err
	}
	out := make([]APIKey, 0, len(keys))
	for _, key := range keys {
		m, err := s.rdb.HGetAll(ctx, key).Result()
		if err == nil && len(m) > 0 {
			apiKey := apiKeyFromMap(key, m)
			if workspace == "" || apiKey.WorkspaceID == workspace {
				out = append(out, apiKey)
			}
		}
	}
	return out, nil
}

func (s *Store) RevokeAPIKey(ctx context.Context, id string) error {
	return s.RevokeAPIKeyInWorkspace(ctx, id, "")
}

func (s *Store) RevokeAPIKeyInWorkspace(ctx context.Context, id, workspace string) error {
	if id == "" {
		return ErrInvalidAPIKey
	}
	m, err := s.rdb.HGetAll(ctx, apiKeyKey(id)).Result()
	if err != nil {
		return err
	}
	if len(m) == 0 || (workspace != "" && workspaceID(m["workspace_id"]) != workspaceID(workspace)) {
		return ErrInvalidAPIKey
	}
	return s.rdb.HSet(ctx, apiKeyKey(id), "revoked_at", time.Now().Unix()).Err()
}

func apiKeyFromMap(key string, m map[string]string) APIKey {
	id := strings.TrimPrefix(key, "el:api_key:")
	var scopes []string
	for _, scope := range strings.Split(m["scopes"], ",") {
		if scope = strings.TrimSpace(scope); scope != "" {
			scopes = append(scopes, scope)
		}
	}
	return APIKey{WorkspaceID: workspaceID(m["workspace_id"]), ID: id, Name: m["name"], Hash: m["hash"], Scopes: scopes, CreatedAt: i64(m["created_at"]), ExpiresAt: i64(m["expires_at"]), RevokedAt: i64(m["revoked_at"])}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Store) SaveCreatedReceipt(ctx context.Context, receipt CreatedReceipt, ttl time.Duration) error {
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, createdReceiptKey(receipt.Token), map[string]any{"link": receipt.Link, "expires_at": receipt.ExpiresAt, "ttl_seconds": receipt.TTLSeconds, "email_status": receipt.EmailStatus})
	pipe.Expire(ctx, createdReceiptKey(receipt.Token), ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) GetCreatedReceipt(ctx context.Context, token string) (CreatedReceipt, error) {
	m, err := s.rdb.HGetAll(ctx, createdReceiptKey(token)).Result()
	if err != nil {
		return CreatedReceipt{}, err
	}
	if len(m) == 0 {
		return CreatedReceipt{}, ErrGone
	}
	return CreatedReceipt{Token: token, Link: m["link"], ExpiresAt: m["expires_at"], TTLSeconds: i64(m["ttl_seconds"]), EmailStatus: m["email_status"]}, nil
}

func throttleKey(scope, identity string) string {
	sum := sha256.Sum256([]byte(scope + ":" + identity))
	return "el:throttle:" + scope + ":" + hex.EncodeToString(sum[:])
}

func (s *Store) FailureLimitExceeded(ctx context.Context, scope, identity string, limit int64) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	count, err := s.rdb.Get(ctx, throttleKey(scope, identity)).Int64()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count >= limit, nil
}

func (s *Store) RegisterFailure(ctx context.Context, scope, identity string, limit int64, window time.Duration) (bool, error) {
	if limit <= 0 || window <= 0 {
		return false, nil
	}
	k := throttleKey(scope, identity)
	count, err := s.rdb.Incr(ctx, k).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := s.rdb.Expire(ctx, k, window).Err(); err != nil {
			return false, err
		}
	}
	return count >= limit, nil
}

func (s *Store) ResetFailures(ctx context.Context, scope, identity string) error {
	return s.rdb.Del(ctx, throttleKey(scope, identity)).Err()
}

func (s *Store) ListAvailableItems(ctx context.Context) ([]Item, error) {
	return s.ListAvailableItemsInWorkspace(ctx, "")
}

func (s *Store) ListAvailableItemsInWorkspace(ctx context.Context, workspace string) ([]Item, error) {
	keys, err := s.scanKeys(ctx, "el:item:*")
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := []Item{}
	for _, k := range keys {
		item, err := parse(s.rdb.HGetAll(ctx, k).Result())
		if err == nil && item.Status == "available" && item.ExpiresAt > now && (workspace == "" || item.WorkspaceID == workspace) {
			out = append(out, item)
		}
	}
	return out, nil
}
func (s *Store) ActiveStorageObjectPaths(ctx context.Context) (map[string]struct{}, error) {
	keys, err := s.scanKeys(ctx, "el:item:*")
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
func (s *Store) MigrateWorkspace(ctx context.Context, from, to string) (int, error) {
	from = workspaceID(from)
	to = workspaceID(to)
	if from == to {
		return 0, nil
	}
	patterns := []string{"el:item:*", "el:upload_request:*", "el:audit:*", "el:user:*", "el:api_key:*"}
	updated := 0
	for _, pattern := range patterns {
		keys, err := s.scanKeys(ctx, pattern)
		if err != nil {
			return updated, err
		}
		for _, key := range keys {
			workspace, err := s.rdb.HGet(ctx, key, "workspace_id").Result()
			if err == redis.Nil {
				workspace = DefaultWorkspaceID
			} else if err != nil {
				return updated, err
			}
			if workspace == from {
				if err := s.rdb.HSet(ctx, key, "workspace_id", to).Err(); err != nil {
					return updated, err
				}
				if strings.HasPrefix(key, "el:user:") {
					username, err := s.rdb.HGet(ctx, key, "username").Result()
					if err == nil {
						role, _ := s.rdb.HGet(ctx, key, "role").Result()
						if err := s.AddWorkspaceMembership(ctx, username, to, role); err != nil {
							return updated, err
						}
					}
				}
				updated++
			}
		}
	}
	return updated, nil
}

func (s *Store) UpdateStorageObjectPath(ctx context.Context, id, path string) error {
	if id == "" || path == "" {
		return ErrGone
	}
	return s.rdb.HSet(ctx, key(id), "storage_object_path", path).Err()
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
	m := map[string]any{"id": req.ID, "workspace_id": workspaceID(req.WorkspaceID), "status": "available", "created_at": req.CreatedAt, "expires_at": req.ExpiresAt, "requested_by": req.RequestedBy, "requester_email": req.RequesterEmail, "recipient_email": req.RecipientEmail, "message": req.Message, "uploaded_item_id": req.UploadedItemID, "uploaded_at": req.UploadedAt}
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
	return s.ListAvailableUploadRequestsInWorkspace(ctx, "")
}

func (s *Store) ListAvailableUploadRequestsInWorkspace(ctx context.Context, workspace string) ([]UploadRequest, error) {
	keys, err := s.scanKeys(ctx, "el:upload_request:*")
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := []UploadRequest{}
	for _, k := range keys {
		req, err := uploadRequestFromMap(s.rdb.HGetAll(ctx, k).Result())
		if err == nil && req.Status == "available" && req.ExpiresAt > now && (workspace == "" || req.WorkspaceID == workspace) {
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
func (s *Store) SaveLegalDocuments(ctx context.Context, documents LegalDocuments) error {
	return s.rdb.HSet(ctx, "el:legal", map[string]any{"privacy": documents.Privacy, "terms": documents.Terms}).Err()
}

func (s *Store) GetLegalDocuments(ctx context.Context) (LegalDocuments, error) {
	m, err := s.rdb.HGetAll(ctx, "el:legal").Result()
	if err != nil {
		return LegalDocuments{}, err
	}
	return LegalDocuments{Privacy: m["privacy"], Terms: m["terms"]}, nil
}

func (s *Store) SaveIntegrationConfig(ctx context.Context, cfg IntegrationConfig) error {
	return s.rdb.HSet(ctx, "el:integration", map[string]any{"microsoft_enabled": boolString(cfg.MicrosoftEnabled), "microsoft_tenant_id": cfg.MicrosoftTenantID, "microsoft_client_id": cfg.MicrosoftClientID, "microsoft_audience": cfg.MicrosoftAudience, "microsoft_authority": cfg.MicrosoftAuthority, "entra_admin_group_name": cfg.EntraAdminGroupName, "entra_admin_group_id": cfg.EntraAdminGroupID, "entra_user_group_name": cfg.EntraUserGroupName, "entra_user_group_id": cfg.EntraUserGroupID, "ad_enabled": boolString(cfg.ADEnabled), "ad_host": cfg.ADHost, "ad_base_dn": cfg.ADBaseDN, "ad_bind_dn": cfg.ADBindDN, "ad_bind_password": cfg.ADBindPassword, "smtp_enabled": boolString(cfg.SMTPEnabled), "smtp_host": cfg.SMTPHost, "smtp_port": cfg.SMTPPort, "smtp_username": cfg.SMTPUsername, "smtp_password": cfg.SMTPPassword, "smtp_from": cfg.SMTPFrom, "graph_enabled": boolString(cfg.GraphEnabled), "graph_tenant_id": cfg.GraphTenantID, "graph_client_id": cfg.GraphClientID, "graph_client_secret": cfg.GraphClientSecret, "graph_sender": cfg.GraphSender}).Err()
}
func (s *Store) GetIntegrationConfig(ctx context.Context) (IntegrationConfig, error) {
	m, err := s.rdb.HGetAll(ctx, "el:integration").Result()
	if err != nil {
		return IntegrationConfig{}, err
	}
	return IntegrationConfig{MicrosoftEnabled: m["microsoft_enabled"] == "true", MicrosoftTenantID: m["microsoft_tenant_id"], MicrosoftClientID: m["microsoft_client_id"], MicrosoftAudience: m["microsoft_audience"], MicrosoftAuthority: m["microsoft_authority"], EntraAdminGroupName: m["entra_admin_group_name"], EntraAdminGroupID: m["entra_admin_group_id"], EntraUserGroupName: m["entra_user_group_name"], EntraUserGroupID: m["entra_user_group_id"], ADEnabled: m["ad_enabled"] == "true", ADHost: m["ad_host"], ADBaseDN: m["ad_base_dn"], ADBindDN: m["ad_bind_dn"], ADBindPassword: m["ad_bind_password"], SMTPEnabled: m["smtp_enabled"] == "true", SMTPHost: m["smtp_host"], SMTPPort: m["smtp_port"], SMTPUsername: m["smtp_username"], SMTPPassword: m["smtp_password"], SMTPFrom: m["smtp_from"], GraphEnabled: m["graph_enabled"] == "true", GraphTenantID: m["graph_tenant_id"], GraphClientID: m["graph_client_id"], GraphClientSecret: m["graph_client_secret"], GraphSender: m["graph_sender"]}, nil
}

func (s *Store) AddAuditEvent(ctx context.Context, event AuditEvent) error {
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().Unix()
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	m := map[string]any{"id": event.ID, "workspace_id": workspaceID(event.WorkspaceID), "created_at": event.CreatedAt, "actor": event.Actor, "ip": event.IP, "event": event.Event, "target": event.Target, "result": event.Result, "details": event.Details}
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, auditKey(event.ID), m)
	pipe.ZAdd(ctx, auditIndexKey, redis.Z{Score: float64(event.CreatedAt), Member: event.ID})
	pipe.Expire(ctx, auditKey(event.ID), 90*24*time.Hour)
	pipe.ZRemRangeByRank(ctx, auditIndexKey, 0, -501)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context, limit int64) ([]AuditEvent, error) {
	return s.ListAuditEventsInWorkspace(ctx, limit, "")
}

func (s *Store) ListAuditEventsInWorkspace(ctx context.Context, limit int64, workspace string) ([]AuditEvent, error) {
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
			event := auditEventFromMap(m)
			if workspace != "" && event.WorkspaceID != workspace {
				continue
			}
			out = append(out, event)
		}
	}
	return out, nil
}

func userMap(user User) map[string]any {
	return map[string]any{"workspace_id": workspaceID(user.WorkspaceID), "username": user.Username, "password_hash": user.PasswordHash, "role": user.Role, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email, "auth_provider": user.AuthProvider, "external_id": user.ExternalID, "created_at": user.CreatedAt}
}
func userFromMap(m map[string]string) User {
	return User{WorkspaceID: workspaceID(m["workspace_id"]), Username: m["username"], PasswordHash: m["password_hash"], Role: m["role"], FirstName: m["first_name"], LastName: m["last_name"], Email: m["email"], AuthProvider: m["auth_provider"], ExternalID: m["external_id"], CreatedAt: i64(m["created_at"])}
}
func auditEventFromMap(m map[string]string) AuditEvent {
	return AuditEvent{WorkspaceID: workspaceID(m["workspace_id"]), ID: m["id"], CreatedAt: i64(m["created_at"]), Actor: m["actor"], IP: m["ip"], Event: m["event"], Target: m["target"], Result: m["result"], Details: m["details"]}
}
func uploadRequestFromMap(m map[string]string, err error) (UploadRequest, error) {
	if err != nil {
		return UploadRequest{}, err
	}
	if len(m) == 0 {
		return UploadRequest{}, ErrGone
	}
	return UploadRequest{WorkspaceID: workspaceID(m["workspace_id"]), ID: m["id"], Status: m["status"], CreatedAt: i64(m["created_at"]), ExpiresAt: i64(m["expires_at"]), RequestedBy: m["requested_by"], RequesterEmail: m["requester_email"], RecipientEmail: m["recipient_email"], Message: m["message"], UploadedItemID: m["uploaded_item_id"], UploadedAt: i64(m["uploaded_at"])}, nil
}
