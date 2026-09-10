// Package repository defines data-access layers.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/helpers" 
	"crm-auth-service/models"
)

// UserRepository defines the database operations for User entities.
type UserRepository interface {
	// FindByEmail searches for a user by email address.
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	// FindByMobile searches for a user by mobile number.
	FindByMobile(ctx context.Context, mobile string) (*models.User, error)
	// FindByID searches for a user by their UUID primary key.
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	// FindBySSOID searches for a user by their SSO provider and subject ID.
	FindBySSOID(ctx context.Context, provider, subjectID string) (*models.User, error)
	// Create persists a new User.
	Create(ctx context.Context, user *models.User) error
	// Update updates an existing User.
	Update(ctx context.Context, user *models.User) error
	// UpdatePasswordAndFirstLogin updates password hash and clears is_first_login.
	UpdatePasswordAndFirstLogin(ctx context.Context, id uuid.UUID, newPasswordHash string) error
	// UpdateEmailVerified sets email_verified to TRUE.
	UpdateEmailVerified(ctx context.Context, id uuid.UUID) error
	// UpdateMobileVerified sets mobile_verified to TRUE.
	UpdateMobileVerified(ctx context.Context, id uuid.UUID) error
	// UpdatePassword updates password hash.
	UpdatePassword(ctx context.Context, id uuid.UUID, newPasswordHash string) error
	// FindActiveUsers returns all active users sorted by name ASC.
	FindActiveUsers(ctx context.Context) ([]*models.User, error)
	// FindAllUsers returns all users in the system.
	FindAllUsers(ctx context.Context) ([]*models.User, error)
	// GetManagedExecutiveIDs returns the UUIDs of executives directly assigned to a manager.
	GetManagedExecutiveIDs(ctx context.Context, managerID uuid.UUID) ([]uuid.UUID, error)
	// GetManagedExecutives returns the executive user objects directly assigned to a manager.
	GetManagedExecutives(ctx context.Context, managerID uuid.UUID) ([]*models.User, error)
	// GetLeaderPermissions returns the list of active delegated permission strings for a leader.
	GetLeaderPermissions(ctx context.Context, leaderID uuid.UUID) ([]string, error)
	// GrantLeaderPermission adds a delegated permission for a leader.
	GrantLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string, grantedBy *uuid.UUID) error
	// RevokeLeaderPermission removes a delegated permission from a leader.
	RevokeLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string) error
	// AssignExecutiveToManager links or unlinks an executive to a manager.
	AssignExecutiveToManager(ctx context.Context, executiveID uuid.UUID, managerID *uuid.UUID) error
	// FindUsersScoped returns users matching the caller's data scope for dropdowns/filtering.
	FindUsersScoped(ctx context.Context, scope helpers.DataScope) ([]*models.User, error)
	// UpdateUserDetails updates user administrative profile (name, email, role, is_active, manager_id).
	UpdateUserDetails(ctx context.Context, id uuid.UUID, name, email, role string, isActive bool, managerID *uuid.UUID) error
	// DeleteUser deactivates or removes a user by ID.
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

// userRepository implements the UserRepository interface using pgxpool.
type userRepository struct {
	db *pgxpool.Pool
}

// NewUserRepository constructs a new instance of UserRepository.
func NewUserRepository(db *pgxpool.Pool) UserRepository {
	return &userRepository{db: db}
}

const selectUserFields = "id, name, email, mobile, password_hash, role, manager_id, is_first_login, password_change_required, email_verified, mobile_verified, mfa_enabled, mfa_method, sso_provider, sso_subject_id, is_active, created_at, updated_at"

func scanUser(row pgx.Row) (*models.User, error) {
	var user models.User
	err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Mobile,
		&user.PasswordHash,
		&user.Role,
		&user.ManagerID,
		&user.IsFirstLogin,
		&user.PasswordChangeRequired,
		&user.EmailVerified,
		&user.MobileVerified,
		&user.MFAEnabled,
		&user.MFAMethod,
		&user.SSOProvider,
		&user.SSOSubjectID,
		&user.IsActive,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

// FindByEmail searches for a user by email address.
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE email = $1"
	row := r.db.QueryRow(ctx, query, email)
	return scanUser(row)
}

// FindByMobile searches for a user by mobile number.
func (r *userRepository) FindByMobile(ctx context.Context, mobile string) (*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE mobile = $1"
	row := r.db.QueryRow(ctx, query, mobile)
	return scanUser(row)
}

// FindByID searches for a user by their UUID primary key.
func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE id = $1"
	row := r.db.QueryRow(ctx, query, id)
	return scanUser(row)
}

// FindBySSOID searches for a user by their SSO provider and subject ID.
func (r *userRepository) FindBySSOID(ctx context.Context, provider, subjectID string) (*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE sso_provider = $1 AND sso_subject_id = $2"
	row := r.db.QueryRow(ctx, query, provider, subjectID)
	return scanUser(row)
}

func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	if user.ID != uuid.Nil {
		query := `INSERT INTO users (id, name, email, mobile, password_hash, role, manager_id, is_first_login, password_change_required, email_verified, mobile_verified, mfa_enabled, mfa_method, sso_provider, sso_subject_id, is_active, created_at, updated_at)
				  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW(), NOW())
				  RETURNING id, created_at, updated_at`
		return r.db.QueryRow(ctx, query, user.ID, user.Name, user.Email, user.Mobile, user.PasswordHash, user.Role, user.ManagerID, user.IsFirstLogin, user.PasswordChangeRequired, user.EmailVerified, user.MobileVerified, user.MFAEnabled, user.MFAMethod, user.SSOProvider, user.SSOSubjectID, user.IsActive).
			Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	}
	query := `INSERT INTO users (name, email, mobile, password_hash, role, manager_id, is_first_login, password_change_required, email_verified, mobile_verified, mfa_enabled, mfa_method, sso_provider, sso_subject_id, is_active, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
			  RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, query, user.Name, user.Email, user.Mobile, user.PasswordHash, user.Role, user.ManagerID, user.IsFirstLogin, user.PasswordChangeRequired, user.EmailVerified, user.MobileVerified, user.MFAEnabled, user.MFAMethod, user.SSOProvider, user.SSOSubjectID, user.IsActive).
		Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

// Update updates an existing User.
func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	query := `UPDATE users 
			   SET name = $1, email = $2, mobile = $3, password_hash = $4, role = $5, manager_id = $6, is_first_login = $7, password_change_required = $8, 
			      email_verified = $9, mobile_verified = $10, mfa_enabled = $11, mfa_method = $12, 
			      sso_provider = $13, sso_subject_id = $14, is_active = $15, updated_at = NOW() 
			  WHERE id = $16`
	_, err := r.db.Exec(ctx, query, user.Name, user.Email, user.Mobile, user.PasswordHash, user.Role, user.ManagerID, user.IsFirstLogin, user.PasswordChangeRequired, user.EmailVerified, user.MobileVerified, user.MFAEnabled, user.MFAMethod, user.SSOProvider, user.SSOSubjectID, user.IsActive, user.ID)
	return err
}

// UpdatePasswordAndFirstLogin updates password hash and clears is_first_login and password_change_required.
func (r *userRepository) UpdatePasswordAndFirstLogin(ctx context.Context, id uuid.UUID, newPasswordHash string) error {
	query := `UPDATE users SET password_hash = $1, is_first_login = FALSE, password_change_required = FALSE, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, newPasswordHash, id)
	return err
}

// UpdateEmailVerified sets email_verified to TRUE.
func (r *userRepository) UpdateEmailVerified(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET email_verified = TRUE, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// UpdateMobileVerified sets mobile_verified to TRUE.
func (r *userRepository) UpdateMobileVerified(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET mobile_verified = TRUE, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// UpdatePassword updates password hash and clears temporary flags.
func (r *userRepository) UpdatePassword(ctx context.Context, id uuid.UUID, newPasswordHash string) error {
	query := `UPDATE users SET password_hash = $1, is_first_login = FALSE, password_change_required = FALSE, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, newPasswordHash, id)
	return err
}

// FindActiveUsers returns all active users sorted by name ASC.
func (r *userRepository) FindActiveUsers(ctx context.Context) ([]*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE is_active = TRUE ORDER BY name ASC"
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// FindAllUsers returns all users in the system sorted by name ASC.
func (r *userRepository) FindAllUsers(ctx context.Context) ([]*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users ORDER BY name ASC"
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// GetManagedExecutiveIDs returns the UUIDs of executives directly assigned to a manager.
func (r *userRepository) GetManagedExecutiveIDs(ctx context.Context, managerID uuid.UUID) ([]uuid.UUID, error) {
	query := "SELECT id FROM users WHERE manager_id = $1 AND role = $2 AND is_active = TRUE"
	rows, err := r.db.Query(ctx, query, managerID, models.RoleSalesExecutive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetManagedExecutives returns executive user objects directly assigned to a manager.
func (r *userRepository) GetManagedExecutives(ctx context.Context, managerID uuid.UUID) ([]*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE manager_id = $1 ORDER BY name ASC"
	rows, err := r.db.Query(ctx, query, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// GetLeaderPermissions returns the list of active delegated permission strings for a leader.
func (r *userRepository) GetLeaderPermissions(ctx context.Context, leaderID uuid.UUID) ([]string, error) {
	query := "SELECT permission FROM leader_delegations WHERE user_id = $1"
	rows, err := r.db.Query(ctx, query, leaderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	if perms == nil {
		perms = []string{}
	}
	return perms, rows.Err()
}

// GrantLeaderPermission adds a delegated permission for a leader.
func (r *userRepository) GrantLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string, grantedBy *uuid.UUID) error {
	query := `INSERT INTO leader_delegations (user_id, permission, granted_by, created_at)
			  VALUES ($1, $2, $3, NOW())
			  ON CONFLICT (user_id, permission) DO NOTHING`
	_, err := r.db.Exec(ctx, query, leaderID, permission, grantedBy)
	return err
}

// RevokeLeaderPermission removes a delegated permission from a leader.
func (r *userRepository) RevokeLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string) error {
	query := "DELETE FROM leader_delegations WHERE user_id = $1 AND permission = $2"
	_, err := r.db.Exec(ctx, query, leaderID, permission)
	return err
}

// AssignExecutiveToManager links or unlinks an executive to a manager.
func (r *userRepository) AssignExecutiveToManager(ctx context.Context, executiveID uuid.UUID, managerID *uuid.UUID) error {
	query := "UPDATE users SET manager_id = $1, updated_at = NOW() WHERE id = $2"
	_, err := r.db.Exec(ctx, query, managerID, executiveID)
	return err
}

// FindUsersScoped returns users matching caller's data scope for dropdowns and filters.
func (r *userRepository) FindUsersScoped(ctx context.Context, scope helpers.DataScope) ([]*models.User, error) {
	if scope.IsUnrestricted {
		return r.FindActiveUsers(ctx)
	}

	if len(scope.AllowedUserIDs) == 0 {
		return []*models.User{}, nil
	}

	query := "SELECT " + selectUserFields + " FROM users WHERE id = ANY($1) AND is_active = TRUE ORDER BY name ASC"
	rows, err := r.db.Query(ctx, query, scope.AllowedUserIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// UpdateUserDetails updates user administrative profile (name, email, role, is_active, manager_id).
func (r *userRepository) UpdateUserDetails(ctx context.Context, id uuid.UUID, name, email, role string, isActive bool, managerID *uuid.UUID) error {
	query := `UPDATE users 
			  SET name = $1, email = $2, role = $3, is_active = $4, manager_id = $5, updated_at = NOW() 
			  WHERE id = $6`
	_, err := r.db.Exec(ctx, query, name, email, role, isActive, managerID, id)
	return err
}

// DeleteUser deactivates or removes a user by ID.
func (r *userRepository) DeleteUser(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET is_active = FALSE, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}