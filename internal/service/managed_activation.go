package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/security"
)

var ErrActivationDenied = errors.New("activation is unavailable or the link has expired")

const activationMarker = "managed_instance_activation"

// ManagedActivation handles first-administrator creation independently of normal
// login. Its transaction marker lets a retry finish a state write after a crash.
type ManagedActivation struct {
	Store       instance.Store
	DB          *sql.DB
	Now         func() time.Time
	AfterCommit func() error
}

// ActivationRecovery is transient material for downloading to the administrator's
// browser. Neither the identity nor a draft of it is written to instance storage.
type ActivationRecovery struct {
	RecoveryFile string `json:"recovery_file"`
	Recipient    string `json:"recipient"`
	Proof        string `json:"proof"`
}

type ActivateInstanceInput struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	RecoveryFile string `json:"recovery_file"`
	Proof        string `json:"proof"`
}

type activationCompletion struct {
	TokenHash string `json:"token_hash"`
	Recipient string `json:"recipient"`
}

func (a *ManagedActivation) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a *ManagedActivation) authenticated(token string) (*instance.State, error) {
	s, err := a.Store.Load()
	if err != nil {
		return nil, err
	}
	if !s.Activation.Accepts(token, a.now()) {
		return nil, ErrActivationDenied
	}
	return s, nil
}

// Reconcile finishes the private state write when the activation transaction
// committed before a process interruption. The database marker is bound to the
// original verifier and is safe to replay after the link has expired.
func (a *ManagedActivation) Reconcile(ctx context.Context) error {
	s, err := a.Store.Load()
	if err != nil || s.Activation == nil {
		return err
	}
	var marker string
	if err := a.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=$1", activationMarker).Scan(&marker); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	var completion activationCompletion
	if json.Unmarshal([]byte(marker), &completion) != nil || completion.TokenHash != s.Activation.TokenHash {
		return ErrActivationDenied
	}
	return a.Store.Update(func(current *instance.State) error {
		if current.Activation == nil {
			return nil
		}
		if current.Activation.TokenHash != completion.TokenHash {
			return ErrActivationDenied
		}
		current.RecoveryRecipient, current.Activation = completion.Recipient, nil
		return nil
	})
}

func recoveryProof(s *instance.State, recipient string) string {
	m := hmac.New(sha256.New, []byte(s.SessionSecret))
	fmt.Fprintf(m, "theia-activation-v1\x00%s\x00%s\x00%s", s.InstanceID, s.Activation.TokenHash, recipient)
	return hex.EncodeToString(m.Sum(nil))
}

// Recovery issues a fresh operator-held recovery identity after checking the
// activation token and the absence of existing users, including disabled users.
func (a *ManagedActivation) Recovery(ctx context.Context, token string) (*ActivationRecovery, error) {
	s, err := a.authenticated(token)
	if err != nil {
		return nil, err
	}
	var count int
	if err := a.DB.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, ErrActivationDenied
	}
	if s.RecoveryRecipient != "" {
		return &ActivationRecovery{Recipient: s.RecoveryRecipient, Proof: recoveryProof(s, s.RecoveryRecipient)}, nil
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	recipient := identity.Recipient().String()
	return &ActivationRecovery{RecoveryFile: fmt.Sprintf("# Theia recovery file. Keep outside the instance host.\n# Instance: %s\n# Recipient: %s\n%s\n", s.InstanceID, recipient, identity), Recipient: recipient, Proof: recoveryProof(s, recipient)}, nil
}

// Complete verifies the actual saved recovery file and atomically creates the
// first administrator, role assignment and completion marker. Existing users
// cannot be bootstrapped again by disabling or deleting their super-admin role.
func (a *ManagedActivation) Complete(ctx context.Context, token string, input ActivateInstanceInput) error {
	s, err := a.authenticated(token)
	if err != nil {
		return err
	}
	identities, err := age.ParseIdentities(strings.NewReader(input.RecoveryFile))
	if err != nil || len(identities) == 0 {
		return fmt.Errorf("saved recovery file is invalid")
	}
	var recipient string
	for _, candidate := range identities {
		if identity, ok := candidate.(*age.X25519Identity); ok {
			value := identity.Recipient().String()
			if (s.RecoveryRecipient == "" || s.RecoveryRecipient == value) && hmac.Equal([]byte(input.Proof), []byte(recoveryProof(s, value))) {
				recipient = value
				break
			}
		}
	}
	if recipient == "" {
		return fmt.Errorf("saved recovery file does not match this activation")
	}
	if !hmac.Equal([]byte(input.Proof), []byte(recoveryProof(s, recipient))) {
		return fmt.Errorf("saved recovery file does not match this activation")
	}
	username, email := strings.TrimSpace(input.Username), strings.TrimSpace(input.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || username == "" || len(username) > 80 || len(email) > 255 {
		return fmt.Errorf("a username and a valid email address are required")
	}
	if err := security.ValidatePasswordPolicy(input.Password); err != nil {
		return err
	}
	hash, err := security.HashPassword(input.Password)
	if err != nil {
		return err
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return err
	}
	var marker string
	err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=$1", activationMarker).Scan(&marker)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	completion := activationCompletion{TokenHash: s.Activation.TokenHash, Recipient: recipient}
	if marker != "" {
		var previous activationCompletion
		if json.Unmarshal([]byte(marker), &previous) != nil || previous != completion {
			return ErrActivationDenied
		}
	} else {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return ErrActivationDenied
		}
		id := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO users(id,username,username_normalized,email,email_normalized,password_hash,display_name,status,must_change_password,created_at,updated_at,password_changed_at)
		VALUES($1,$2,$3,$4,$5,$6,$2,'active',false,$7,$7,$7)`, id, username, normalizeLoginIdentifier(username), email, normalizeLoginIdentifier(email), hash, a.now()); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, "INSERT INTO user_roles(user_id,role_id,created_at) SELECT $1,id,$2 FROM roles WHERE name='super_admin'", id, a.now())
		if err != nil {
			return err
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return fmt.Errorf("first-administrator role is missing")
		}
		data, _ := json.Marshal(completion)
		if _, err := tx.ExecContext(ctx, "INSERT INTO settings(key,value,updated_at) VALUES($1,$2,$3)", activationMarker, string(data), a.now()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO audit_logs(id,actor_user_id,target_user_id,action,resource,resource_id,metadata_json,created_at) VALUES($1,$2,$2,'instance.activated','auth_user',$2,'{}',$3)", uuid.NewString(), id, a.now()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if a.AfterCommit != nil {
		if err := a.AfterCommit(); err != nil {
			return err
		}
	}
	return a.Store.Update(func(current *instance.State) error {
		if current.InstanceID != s.InstanceID || current.Activation == nil || current.Activation.TokenHash != completion.TokenHash {
			return ErrActivationDenied
		}
		if current.RecoveryRecipient != "" && current.RecoveryRecipient != recipient {
			return ErrActivationDenied
		}
		current.RecoveryRecipient, current.Activation = recipient, nil
		return nil
	})
}
