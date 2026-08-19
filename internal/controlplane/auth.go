package controlplane

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/cache"
	security "github.com/starprince1234/Nebula-api/internal/infrastructure/crypto"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/user"
)

const (
	PurposeStudentRegistration = "student_registration"
	PurposeMentorRegistration  = "mentor_registration"
	PurposePasswordReset       = "password_reset"
)

var dummyPasswordHash = func() string {
	hash, _ := security.HashPassword("not-a-real-user-password")
	return hash
}()

func (s *Service) SendRegistrationCode(ctx context.Context, email, purpose string) error {
	if purpose != PurposeStudentRegistration && purpose != PurposeMentorRegistration {
		return domain.NewError(domain.CodeValidation, "invalid verification purpose")
	}
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return err
	}
	exists, err := s.db.User.Query().Where(user.EmailEQ(normalized)).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query user", err)
	}
	if exists {
		return domain.NewError(domain.CodeEmailRegistered, "email is already registered")
	}
	return s.sendCode(ctx, normalized, purpose)
}

func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return err
	}
	exists, err := s.db.User.Query().Where(user.EmailEQ(normalized), user.StatusNEQ(user.StatusDisabled)).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query user", err)
	}
	if !exists {
		return nil
	}
	return s.sendCode(ctx, normalized, PurposePasswordReset)
}

func (s *Service) sendCode(ctx context.Context, email, purpose string) error {
	code, err := randomCode()
	if err != nil {
		return domain.WrapError("INTERNAL_ERROR", "generate verification code", err)
	}
	hash := s.security.HashAuthState(email + "|" + purpose + "|" + code)
	if err := s.cache.PutVerification(ctx, email, purpose, hash, s.config.VerificationTTL, s.config.SendCooldown); err != nil {
		if err == cache.ErrRateLimited {
			return domain.NewError(domain.CodeRateLimited, "verification code was sent recently")
		}
		return domain.WrapError(domain.CodeDependencyUnavailable, "store verification code", err)
	}
	subject := "Nebula verification code"
	body := fmt.Sprintf("Your Nebula verification code is %s. It expires in %d minutes.", code, int(s.config.VerificationTTL.Minutes()))
	if err := s.mailer.Send(ctx, email, subject, body); err != nil {
		_ = s.cache.DeleteVerification(ctx, email, purpose)
		return domain.WrapError(domain.CodeDependencyUnavailable, "send verification email", err)
	}
	return nil
}

func (s *Service) Register(ctx context.Context, role, name, email, password, code string) (UserView, error) {
	if role != string(user.RoleStudent) && role != string(user.RoleMentor) {
		return UserView{}, domain.NewError(domain.CodeValidation, "invalid registration role")
	}
	name, err := ValidateName(name, 64)
	if err != nil {
		return UserView{}, err
	}
	email, err = NormalizeEmail(email)
	if err != nil {
		return UserView{}, err
	}
	purpose := PurposeStudentRegistration
	if role == string(user.RoleMentor) {
		purpose = PurposeMentorRegistration
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return UserView{}, domain.NewError(domain.CodeValidation, err.Error())
	}
	hash := s.security.HashAuthState(email + "|" + purpose + "|" + strings.TrimSpace(code))
	if err := s.consumeVerification(ctx, email, purpose, hash); err != nil {
		return UserView{}, err
	}
	created, err := s.db.User.Create().
		SetName(name).
		SetEmail(email).
		SetPasswordHash(passwordHash).
		SetRole(user.Role(role)).
		SetStatus(user.StatusActive).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return UserView{}, domain.NewError(domain.CodeEmailRegistered, "email is already registered")
	}
	if err != nil {
		return UserView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create user", err)
	}
	return userView(created), nil
}

func (s *Service) Login(ctx context.Context, email, password string) (SessionView, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return SessionView{}, domain.NewError(domain.CodeInvalidCredentials, "invalid email or password")
	}
	account, err := s.db.User.Query().Where(user.EmailEQ(normalized)).Only(ctx)
	if ent.IsNotFound(err) {
		_ = security.VerifyPassword(dummyPasswordHash, password)
		return SessionView{}, domain.NewError(domain.CodeInvalidCredentials, "invalid email or password")
	}
	if err != nil {
		return SessionView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query user", err)
	}
	if !security.VerifyPassword(account.PasswordHash, password) {
		return SessionView{}, domain.NewError(domain.CodeInvalidCredentials, "invalid email or password")
	}
	if account.Status != user.StatusActive {
		return SessionView{}, domain.NewError(domain.CodeAccountDisabled, "account is not active")
	}
	return s.issueSession(ctx, account, "")
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (SessionView, error) {
	if rawRefreshToken == "" {
		return SessionView{}, domain.NewError(domain.CodeAuthenticationRequired, "refresh token is required")
	}
	oldHash := cache.HashKey(s.security.HashAuthState(rawRefreshToken))
	session, err := s.cache.GetSession(ctx, oldHash)
	if err != nil {
		return SessionView{}, domain.NewError(domain.CodeAuthenticationRequired, "invalid refresh token")
	}
	userID, err := uuid.Parse(session.UserID)
	if err != nil {
		return SessionView{}, domain.NewError(domain.CodeAuthenticationRequired, "invalid refresh token")
	}
	account, err := s.db.User.Query().Where(user.IDEQ(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return SessionView{}, domain.NewError(domain.CodeAuthenticationRequired, "invalid refresh token")
	}
	if err != nil {
		return SessionView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query user", err)
	}
	if account.Status != user.StatusActive {
		return SessionView{}, domain.NewError(domain.CodeAccountDisabled, "account is not active")
	}
	newRaw, err := s.security.NewOpaqueToken("neb_rt_", 32)
	if err != nil {
		return SessionView{}, domain.WrapError("INTERNAL_ERROR", "generate refresh token", err)
	}
	newHash := cache.HashKey(s.security.HashAuthState(newRaw))
	newSession := cache.Session{UserID: account.ID.String(), FamilyID: session.FamilyID}
	if err := s.cache.RotateSession(ctx, oldHash, newHash, newSession, s.config.RefreshTTL); err != nil {
		return SessionView{}, domain.NewError(domain.CodeAuthenticationRequired, "invalid refresh token")
	}
	access, expiresAt, err := s.security.IssueAccessToken(account.ID, string(account.Role))
	if err != nil {
		return SessionView{}, domain.WrapError("INTERNAL_ERROR", "issue access token", err)
	}
	return SessionView{
		AccessToken: access, TokenType: "Bearer",
		ExpiresIn:    int64(time.Until(expiresAt).Seconds()),
		RefreshToken: newRaw, User: userView(account),
	}, nil
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	hash := cache.HashKey(s.security.HashAuthState(rawRefreshToken))
	if err := s.cache.DeleteSession(ctx, hash); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "delete refresh session", err)
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return err
	}
	passwordHash, err := security.HashPassword(newPassword)
	if err != nil {
		return domain.NewError(domain.CodeValidation, err.Error())
	}
	hash := s.security.HashAuthState(normalized + "|" + PurposePasswordReset + "|" + strings.TrimSpace(code))
	if err := s.consumeVerification(ctx, normalized, PurposePasswordReset, hash); err != nil {
		return err
	}
	account, err := s.db.User.Query().Where(user.EmailEQ(normalized)).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.NewError(domain.CodeVerificationInvalid, "invalid or expired verification code")
	}
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query user", err)
	}
	if _, err := account.Update().SetPasswordHash(passwordHash).Save(ctx); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "update password", err)
	}
	if err := s.cache.RevokeUserSessions(ctx, account.ID.String(), s.config.RefreshTTL); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "revoke refresh sessions", err)
	}
	return nil
}

func (s *Service) CurrentUser(ctx context.Context, identity Identity) (UserView, error) {
	account, err := s.db.User.Query().Where(user.IDEQ(identity.UserID)).Only(ctx)
	if ent.IsNotFound(err) {
		return UserView{}, domain.NewError(domain.CodeAuthenticationRequired, "user no longer exists")
	}
	if err != nil {
		return UserView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query user", err)
	}
	if account.Status != user.StatusActive {
		return UserView{}, domain.NewError(domain.CodeAccountDisabled, "account is not active")
	}
	return userView(account), nil
}

func (s *Service) InviteTeacher(ctx context.Context, inviter Identity, email string) error {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return err
	}
	existing, err := s.db.User.Query().Where(user.EmailEQ(normalized)).Only(ctx)
	if err == nil && existing.Status != user.StatusPendingInvite {
		return domain.NewError(domain.CodeEmailRegistered, "email is already registered")
	}
	if err != nil && !ent.IsNotFound(err) {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query teacher invitation", err)
	}
	if ent.IsNotFound(err) {
		unusablePassword, tokenErr := s.security.NewOpaqueToken("", 32)
		if tokenErr != nil {
			return domain.WrapError("INTERNAL_ERROR", "generate pending password", tokenErr)
		}
		passwordHash, hashErr := security.HashPassword(unusablePassword)
		if hashErr != nil {
			return domain.WrapError("INTERNAL_ERROR", "hash pending password", hashErr)
		}
		existing, err = s.db.User.Create().
			SetName("Pending Teacher").
			SetEmail(normalized).
			SetPasswordHash(passwordHash).
			SetRole(user.RoleTeacher).
			SetStatus(user.StatusPendingInvite).
			Save(ctx)
		if ent.IsConstraintError(err) {
			return domain.NewError(domain.CodeEmailRegistered, "email is already registered")
		}
		if err != nil {
			return domain.WrapError(domain.CodeDependencyUnavailable, "create pending teacher", err)
		}
	}
	token, err := s.security.NewOpaqueToken("neb_inv_", 32)
	if err != nil {
		return domain.WrapError("INTERNAL_ERROR", "generate teacher invitation", err)
	}
	hash := cache.HashKey(s.security.HashAuthState(token))
	if err := s.cache.PutInvitation(ctx, hash, cache.Invitation{
		Email: normalized, InviterID: inviter.UserID.String(),
	}, s.config.InvitationTTL); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "store teacher invitation", err)
	}
	link := strings.TrimRight(s.config.PublicAppURL, "/") + "/activate-teacher#token=" + token
	body := "You were invited as a Nebula teacher. Activate the account at: " + link
	if err := s.mailer.Send(ctx, normalized, "Nebula teacher invitation", body); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "send teacher invitation", err)
	}
	_ = existing
	return nil
}

func (s *Service) ActivateTeacher(ctx context.Context, token, name, password string) (UserView, error) {
	name, err := ValidateName(name, 64)
	if err != nil {
		return UserView{}, err
	}
	hash := cache.HashKey(s.security.HashAuthState(strings.TrimSpace(token)))
	invitation, err := s.cache.GetInvitation(ctx, hash)
	if err != nil {
		return UserView{}, domain.NewError(domain.CodeInvitationInvalid, "invalid or expired invitation")
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return UserView{}, domain.NewError(domain.CodeValidation, err.Error())
	}
	account, err := s.db.User.Query().Where(
		user.EmailEQ(invitation.Email),
		user.RoleEQ(user.RoleTeacher),
		user.StatusEQ(user.StatusPendingInvite),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return UserView{}, domain.NewError(domain.CodeInvitationInvalid, "invalid or expired invitation")
	}
	if err != nil {
		return UserView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query invited teacher", err)
	}
	account, err = account.Update().
		SetName(name).
		SetPasswordHash(passwordHash).
		SetStatus(user.StatusActive).
		Save(ctx)
	if err != nil {
		return UserView{}, domain.WrapError(domain.CodeDependencyUnavailable, "activate teacher", err)
	}
	_ = s.cache.DeleteInvitation(ctx, hash)
	return userView(account), nil
}

func (s *Service) BootstrapTeacher(ctx context.Context, name, email, password string) error {
	if name == "" && email == "" && password == "" {
		return nil
	}
	exists, err := s.db.User.Query().Where(user.RoleEQ(user.RoleTeacher)).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query bootstrap teacher", err)
	}
	if exists {
		return nil
	}
	name, err = ValidateName(name, 64)
	if err != nil {
		return err
	}
	email, err = NormalizeEmail(email)
	if err != nil {
		return err
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return domain.NewError(domain.CodeValidation, err.Error())
	}
	_, err = s.db.User.Create().
		SetName(name).
		SetEmail(email).
		SetPasswordHash(passwordHash).
		SetRole(user.RoleTeacher).
		SetStatus(user.StatusActive).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return domain.NewError(domain.CodeEmailRegistered, "bootstrap teacher email already exists")
	}
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "create bootstrap teacher", err)
	}
	return nil
}

func (s *Service) issueSession(ctx context.Context, account *ent.User, familyID string) (SessionView, error) {
	refreshToken, err := s.security.NewOpaqueToken("neb_rt_", 32)
	if err != nil {
		return SessionView{}, domain.WrapError("INTERNAL_ERROR", "generate refresh token", err)
	}
	if familyID == "" {
		familyID = uuid.Must(uuid.NewV7()).String()
	}
	hash := cache.HashKey(s.security.HashAuthState(refreshToken))
	if err := s.cache.CreateSession(ctx, hash, cache.Session{
		UserID: account.ID.String(), FamilyID: familyID,
	}, s.config.RefreshTTL); err != nil {
		return SessionView{}, domain.WrapError(domain.CodeDependencyUnavailable, "store refresh session", err)
	}
	access, expiresAt, err := s.security.IssueAccessToken(account.ID, string(account.Role))
	if err != nil {
		return SessionView{}, domain.WrapError("INTERNAL_ERROR", "issue access token", err)
	}
	return SessionView{
		AccessToken: access, TokenType: "Bearer",
		ExpiresIn:    int64(time.Until(expiresAt).Seconds()),
		RefreshToken: refreshToken, User: userView(account),
	}, nil
}

func (s *Service) consumeVerification(ctx context.Context, email, purpose string, hash []byte) error {
	err := s.cache.ConsumeVerification(ctx, email, purpose, hash, s.config.MaxAttempts, s.config.Lockout)
	switch err {
	case nil:
		return nil
	case cache.ErrVerificationLocked:
		return domain.NewError(domain.CodeVerificationLocked, "verification attempts are locked")
	case cache.ErrVerificationMissing:
		return domain.NewError(domain.CodeVerificationExpired, "verification code expired")
	case cache.ErrVerificationInvalid:
		return domain.NewError(domain.CodeVerificationInvalid, "invalid verification code")
	default:
		return domain.WrapError(domain.CodeDependencyUnavailable, "verify code", err)
	}
}

func randomCode() (string, error) {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}
