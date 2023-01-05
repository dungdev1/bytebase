package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/pkg/errors"

	"github.com/bytebase/bytebase/api"
	"github.com/bytebase/bytebase/common"
	"github.com/bytebase/bytebase/metric"
)

// databaseRaw is the store model for an Database.
// Fields have exactly the same meanings as Database.
type databaseRaw struct {
	ID int

	// Standard fields
	CreatorID int
	CreatedTs int64
	UpdaterID int
	UpdatedTs int64

	// Related fields
	ProjectID      int
	InstanceID     int
	SourceBackupID int

	// Domain specific fields
	Name                 string
	CharacterSet         string
	Collation            string
	SchemaVersion        string
	SyncStatus           api.SyncStatus
	LastSuccessfulSyncTs int64
}

// toDatabase creates an instance of Database based on the databaseRaw.
// This is intended to be called when we need to compose an Database relationship.
func (raw *databaseRaw) toDatabase() *api.Database {
	return &api.Database{
		ID: raw.ID,

		// Standard fields
		CreatorID: raw.CreatorID,
		CreatedTs: raw.CreatedTs,
		UpdaterID: raw.UpdaterID,
		UpdatedTs: raw.UpdatedTs,

		// Related fields
		ProjectID:      raw.ProjectID,
		InstanceID:     raw.InstanceID,
		SourceBackupID: raw.SourceBackupID,

		// Domain specific fields
		Name:                 raw.Name,
		CharacterSet:         raw.CharacterSet,
		Collation:            raw.Collation,
		SchemaVersion:        raw.SchemaVersion,
		SyncStatus:           raw.SyncStatus,
		LastSuccessfulSyncTs: raw.LastSuccessfulSyncTs,
	}
}

// FindDatabase finds a list of Database instances.
func (s *Store) FindDatabase(ctx context.Context, find *api.DatabaseFind) ([]*api.Database, error) {
	databaseRawList, err := s.findDatabaseRaw(ctx, find)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find Database list with DatabaseFind[%+v]", find)
	}
	var databaseList []*api.Database
	for _, raw := range databaseRawList {
		database, err := s.composeDatabase(ctx, raw)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to compose Database with databaseRaw[%+v]", raw)
		}
		databaseList = append(databaseList, database)
	}

	// If no specified instance, filter out databases belonging to archived instances.
	if find.InstanceID == nil {
		var filteredList []*api.Database
		for _, database := range databaseList {
			if i := database.Instance; i == nil || i.RowStatus == api.Archived {
				continue
			}
			filteredList = append(filteredList, database)
		}
		databaseList = filteredList
	}

	return databaseList, nil
}

// GetDatabase gets an instance of Database.
func (s *Store) GetDatabase(ctx context.Context, find *api.DatabaseFind) (*api.Database, error) {
	databaseRaw, err := s.getDatabaseRaw(ctx, find)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get Database with DatabaseFind[%+v]", find)
	}
	if databaseRaw == nil {
		return nil, nil
	}
	database, err := s.composeDatabase(ctx, databaseRaw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compose Database with databaseRaw[%+v]", databaseRaw)
	}
	return database, nil
}

// PatchDatabase patches an instance of Database.
func (s *Store) PatchDatabase(ctx context.Context, patch *api.DatabasePatch) (*api.Database, error) {
	databaseRaw, err := s.patchDatabaseRaw(ctx, patch)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to patch Database with DatabasePatch[%+v]", patch)
	}
	database, err := s.composeDatabase(ctx, databaseRaw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compose Database with databaseRaw[%+v]", databaseRaw)
	}
	return database, nil
}

// CountDatabaseGroupByBackupScheduleAndEnabled counts database, group by backup schedule and enabled.
func (s *Store) CountDatabaseGroupByBackupScheduleAndEnabled(ctx context.Context) ([]*metric.DatabaseCountMetric, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		WITH database_backup_policy AS (
			SELECT db.id AS database_id, backup_policy.payload AS payload
			FROM db, instance LEFT JOIN (
				SELECT resource_id, payload
				FROM policy
				WHERE type = 'bb.policy.backup-plan'
			) AS backup_policy ON instance.environment_id = backup_policy.resource_id
			WHERE db.instance_id = instance.id
		), database_backup_setting AS(
			SELECT db.id AS database_id, backup_setting.enabled AS enabled
			FROM db LEFT JOIN backup_setting ON db.id = backup_setting.database_id
		)
		SELECT database_backup_policy.payload, database_backup_setting.enabled, COUNT(*)
		FROM database_backup_policy FULL JOIN database_backup_setting
			ON database_backup_policy.database_id = database_backup_setting.database_id
		GROUP BY database_backup_policy.payload, database_backup_setting.enabled
		`)
	if err != nil {
		return nil, FormatError(err)
	}
	defer rows.Close()

	var databaseCountMetricList []*metric.DatabaseCountMetric
	for rows.Next() {
		var optionalPayload sql.NullString
		var optionalEnabled sql.NullBool
		var count int
		if err := rows.Scan(&optionalPayload, &optionalEnabled, &count); err != nil {
			return nil, FormatError(err)
		}
		var backupPlanPolicySchedule *api.BackupPlanPolicySchedule
		if optionalPayload.Valid {
			backupPlanPolicy, err := api.UnmarshalBackupPlanPolicy(optionalPayload.String)
			if err != nil {
				return nil, FormatError(err)
			}
			backupPlanPolicySchedule = &backupPlanPolicy.Schedule
		}
		var enabled *bool
		if optionalEnabled.Valid {
			enabled = &optionalEnabled.Bool
		}
		databaseCountMetricList = append(databaseCountMetricList, &metric.DatabaseCountMetric{
			BackupPlanPolicySchedule: backupPlanPolicySchedule,
			BackupSettingEnabled:     enabled,
			Count:                    count,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, FormatError(err)
	}

	return databaseCountMetricList, nil
}

//
// private functions
//

func (s *Store) composeDatabase(ctx context.Context, raw *databaseRaw) (*api.Database, error) {
	db := raw.toDatabase()

	creator, err := s.GetPrincipalByID(ctx, db.CreatorID)
	if err != nil {
		return nil, err
	}
	db.Creator = creator

	updater, err := s.GetPrincipalByID(ctx, db.UpdaterID)
	if err != nil {
		return nil, err
	}
	db.Updater = updater

	project, err := s.GetProjectByID(ctx, db.ProjectID)
	if err != nil {
		return nil, err
	}
	db.Project = project

	instance, err := s.GetInstanceByID(ctx, db.InstanceID)
	if err != nil {
		return nil, err
	}
	db.Instance = instance

	if db.SourceBackupID != 0 {
		sourceBackup, err := s.GetBackupByID(ctx, db.SourceBackupID)
		if err != nil {
			return nil, err
		}
		db.SourceBackup = sourceBackup
	}

	// For now, only wildcard(*) database has data sources and we disallow it to be returned to the client.
	// So we set this value to an empty array until we need to develop a data source for a non-wildcard database.
	db.DataSourceList = []*api.DataSource{}

	rowStatus := api.Normal
	labelList, err := s.findDatabaseLabel(ctx, &api.DatabaseLabelFind{
		DatabaseID: db.ID,
		RowStatus:  &rowStatus,
	})
	if err != nil {
		return nil, err
	}

	// Since tenants are identified by labels in deployment config, we need an environment
	// label to identify tenants from different environment in a schema update deployment.
	// If we expose the environment label concept in the deployment config, it should look consistent in the label API.

	// Each database instance is created under a particular environment.
	// The value of bb.environment is identical to the name of the environment.

	labelList = append(labelList, &api.DatabaseLabel{
		Key:   api.EnvironmentLabelKey,
		Value: db.Instance.Environment.Name,
	})

	labels, err := json.Marshal(labelList)
	if err != nil {
		return nil, err
	}
	db.Labels = string(labels)

	return db, nil
}

// findDatabaseRaw retrieves a list of databases based on find.
func (s *Store) findDatabaseRaw(ctx context.Context, find *api.DatabaseFind) ([]*databaseRaw, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	list, err := s.findDatabaseImpl(ctx, tx, find)
	if err != nil {
		return nil, err
	}

	if err == nil {
		for _, database := range list {
			if err := s.cache.UpsertCache(databaseCacheNamespace, database.ID, database); err != nil {
				return nil, err
			}
		}
	}

	return list, nil
}

// getDatabaseRaw retrieves a single database based on find.
// Returns ECONFLICT if finding more than 1 matching records.
func (s *Store) getDatabaseRaw(ctx context.Context, find *api.DatabaseFind) (*databaseRaw, error) {
	if find.ID != nil {
		databaseRaw := &databaseRaw{}
		has, err := s.cache.FindCache(databaseCacheNamespace, *find.ID, databaseRaw)
		if err != nil {
			return nil, err
		}
		if has {
			return databaseRaw, nil
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	list, err := s.findDatabaseImpl(ctx, tx, find)
	if err != nil {
		return nil, err
	}

	if len(list) == 0 {
		return nil, nil
	} else if len(list) > 1 {
		return nil, &common.Error{Code: common.Conflict, Err: errors.Errorf("found %d databases with filter %+v, expect 1. ", len(list), find)}
	}

	if err := s.cache.UpsertCache(databaseCacheNamespace, list[0].ID, list[0]); err != nil {
		return nil, err
	}

	return list[0], nil
}

// patchDatabaseRaw updates an existing database by ID.
// Returns ENOTFOUND if database does not exist.
func (s *Store) patchDatabaseRaw(ctx context.Context, patch *api.DatabasePatch) (*databaseRaw, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	database, err := s.patchDatabaseImpl(ctx, tx, patch)
	if err != nil {
		return nil, FormatError(err)
	}

	if err := tx.Commit(); err != nil {
		return nil, FormatError(err)
	}

	if err := s.cache.UpsertCache(databaseCacheNamespace, database.ID, database); err != nil {
		return nil, err
	}

	return database, nil
}

func (*Store) findDatabaseImpl(ctx context.Context, tx *Tx, find *api.DatabaseFind) ([]*databaseRaw, error) {
	// Build WHERE clause.
	where, args := []string{"1 = 1"}, []interface{}{}
	if v := find.ID; v != nil {
		where, args = append(where, fmt.Sprintf("id = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.InstanceID; v != nil {
		where, args = append(where, fmt.Sprintf("instance_id = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.ProjectID; v != nil {
		where, args = append(where, fmt.Sprintf("project_id = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.Name; v != nil {
		where, args = append(where, fmt.Sprintf("name = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.SyncStatus; v != nil {
		where, args = append(where, fmt.Sprintf("sync_status = $%d", len(args)+1)), append(args, *v)
	}
	if !find.IncludeAllDatabase {
		where = append(where, "name != '"+api.AllDatabaseName+"'")
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT
			id,
			creator_id,
			created_ts,
			updater_id,
			updated_ts,
			instance_id,
			project_id,
			source_backup_id,
			name,
			character_set,
			"collation",
			sync_status,
			last_successful_sync_ts,
			schema_version
		FROM db
		WHERE `+strings.Join(where, " AND "),
		args...,
	)
	if err != nil {
		return nil, FormatError(err)
	}
	defer rows.Close()

	// Iterate over result set and deserialize rows into databaseRawList.
	var databaseRawList []*databaseRaw
	for rows.Next() {
		var databaseRaw databaseRaw
		var nullSourceBackupID sql.NullInt64
		if err := rows.Scan(
			&databaseRaw.ID,
			&databaseRaw.CreatorID,
			&databaseRaw.CreatedTs,
			&databaseRaw.UpdaterID,
			&databaseRaw.UpdatedTs,
			&databaseRaw.InstanceID,
			&databaseRaw.ProjectID,
			&nullSourceBackupID,
			&databaseRaw.Name,
			&databaseRaw.CharacterSet,
			&databaseRaw.Collation,
			&databaseRaw.SyncStatus,
			&databaseRaw.LastSuccessfulSyncTs,
			&databaseRaw.SchemaVersion,
		); err != nil {
			return nil, FormatError(err)
		}
		if nullSourceBackupID.Valid {
			databaseRaw.SourceBackupID = int(nullSourceBackupID.Int64)
		}

		databaseRawList = append(databaseRawList, &databaseRaw)
	}
	if err := rows.Err(); err != nil {
		return nil, FormatError(err)
	}

	return databaseRawList, nil
}

// patchDatabaseImpl updates a database by ID. Returns the new state of the database after update.
func (*Store) patchDatabaseImpl(ctx context.Context, tx *Tx, patch *api.DatabasePatch) (*databaseRaw, error) {
	// Build UPDATE clause.
	set, args := []string{"updater_id = $1"}, []interface{}{patch.UpdaterID}
	if v := patch.ProjectID; v != nil {
		set, args = append(set, fmt.Sprintf("project_id = $%d", len(args)+1)), append(args, *v)
	}
	if v := patch.SourceBackupID; v != nil {
		set, args = append(set, fmt.Sprintf("source_backup_id = $%d", len(args)+1)), append(args, *v)
	}
	if v := patch.SchemaVersion; v != nil {
		set, args = append(set, fmt.Sprintf("schema_version = $%d", len(args)+1)), append(args, *v)
	}
	if v := patch.SyncStatus; v != nil {
		set, args = append(set, fmt.Sprintf("sync_status = $%d", len(args)+1)), append(args, api.SyncStatus(*v))
	}
	if v := patch.LastSuccessfulSyncTs; v != nil {
		set, args = append(set, fmt.Sprintf("last_successful_sync_ts = $%d", len(args)+1)), append(args, *v)
	}

	args = append(args, patch.ID)

	var databaseRaw databaseRaw
	var nullSourceBackupID sql.NullInt64
	// Execute update query with RETURNING.
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
		UPDATE db
		SET `+strings.Join(set, ", ")+`
		WHERE id = $%d
		RETURNING
			id,
			creator_id,
			created_ts,
			updater_id,
			updated_ts,
			instance_id,
			project_id,
			source_backup_id,
			name,
			character_set,
			"collation",
			sync_status,
			last_successful_sync_ts,
			schema_version
	`, len(args)),
		args...,
	).Scan(
		&databaseRaw.ID,
		&databaseRaw.CreatorID,
		&databaseRaw.CreatedTs,
		&databaseRaw.UpdaterID,
		&databaseRaw.UpdatedTs,
		&databaseRaw.InstanceID,
		&databaseRaw.ProjectID,
		&nullSourceBackupID,
		&databaseRaw.Name,
		&databaseRaw.CharacterSet,
		&databaseRaw.Collation,
		&databaseRaw.SyncStatus,
		&databaseRaw.LastSuccessfulSyncTs,
		&databaseRaw.SchemaVersion,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, &common.Error{Code: common.NotFound, Err: errors.Errorf("database ID not found: %d", patch.ID)}
		}
		return nil, FormatError(err)
	}
	if nullSourceBackupID.Valid {
		databaseRaw.SourceBackupID = int(nullSourceBackupID.Int64)
	}
	return &databaseRaw, nil
}

// DatabaseMessage is the message for database.
type DatabaseMessage struct {
	UID           int
	ProjectID     string
	EnvironmentID string
	InstanceID    string

	DatabaseName         string
	CharacterSet         string
	Collation            string
	SyncState            api.SyncStatus
	SuccessfulSyncTimeTs int64
	SchemaVersion        string
	Labels               map[string]string
}

// FindDatabaseMessage is the message for finding databases.
type FindDatabaseMessage struct {
	ProjectID     *string
	EnvironmentID *string
	InstanceID    *string
	DatabaseName  *string
	UID           *int
}

// GetDatabaseV2 gets a database.
func (s *Store) GetDatabaseV2(ctx context.Context, find *FindDatabaseMessage) (*DatabaseMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	databases, err := s.listDatabaseImplV2(ctx, tx, find)
	if err != nil {
		return nil, err
	}
	if len(databases) == 0 {
		return nil, nil
	}
	if len(databases) > 1 {
		return nil, &common.Error{Code: common.Conflict, Err: errors.Errorf("found %d database with filter %+v, expect 1", len(databases), find)}
	}
	database := databases[0]

	if err := tx.Commit(); err != nil {
		return nil, FormatError(err)
	}

	return database, nil
}

// ListDatabases lists all databases.
func (s *Store) ListDatabases(ctx context.Context, find *FindDatabaseMessage) ([]*DatabaseMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	databases, err := s.listDatabaseImplV2(ctx, tx, find)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, FormatError(err)
	}

	return databases, nil
}

// CreateDatabaseDefault creates a new database with charset, collation only in the default project.
func (s *Store) CreateDatabaseDefault(ctx context.Context, create *DatabaseMessage) error {
	instance, err := s.GetInstanceV2(ctx, &FindInstanceMessage{EnvironmentID: &create.EnvironmentID, ResourceID: &create.InstanceID})
	if err != nil {
		return err
	}
	if instance == nil {
		return errors.Errorf("instance %q not found", create.InstanceID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FormatError(err)
	}
	defer tx.Rollback()

	if _, err := s.createDatabaseDefaultImpl(ctx, tx, instance.UID, create); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return FormatError(err)
	}

	return nil
}

// createDatabaseDefault only creates a default database with charset, collation only in the default project.
func (*Store) createDatabaseDefaultImpl(ctx context.Context, tx *Tx, instanceUID int, create *DatabaseMessage) (int, error) {
	// We will do on conflict update the column updater_id for returning the ID because on conflict do nothing will not return anything.
	query := `
		INSERT INTO db (
			creator_id,
			updater_id,
			instance_id,
			project_id,
			name,
			character_set,
			"collation",
			sync_status,
			last_successful_sync_ts,
			schema_version
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (instance_id, name) DO UPDATE SET
			updater_id = EXCLUDED.updater_id
		RETURNING id`
	var databaseUID int
	if err := tx.QueryRowContext(ctx, query,
		api.SystemBotID,
		api.SystemBotID,
		instanceUID,
		api.DefaultProjectID,
		create.DatabaseName,
		create.CharacterSet,
		create.Collation,
		api.OK,
		0,  /* last_successful_sync_ts */
		"", /* schema_version */
	).Scan(
		&databaseUID,
	); err != nil {
		return 0, FormatError(err)
	}
	return databaseUID, nil
}

// UpsertDatabase upserts a database.
func (s *Store) UpsertDatabase(ctx context.Context, create *DatabaseMessage) (*DatabaseMessage, error) {
	project, err := s.GetProjectV2(ctx, &FindProjectMessage{ResourceID: &create.ProjectID})
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, errors.Errorf("project %q not found", create.ProjectID)
	}
	instance, err := s.GetInstanceV2(ctx, &FindInstanceMessage{EnvironmentID: &create.EnvironmentID, ResourceID: &create.InstanceID})
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, errors.Errorf("instance %q not found", create.InstanceID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// We will do on conflict update the column updater_id for returning the ID because on conflict do nothing will not return anything.
	query := `
		INSERT INTO db (
			creator_id,
			updater_id,
			instance_id,
			project_id,
			name,
			character_set,
			"collation",
			sync_status,
			last_successful_sync_ts,
			schema_version
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (instance_id, name) DO UPDATE SET
			project_id = EXCLUDED.project_id,
			name = EXCLUDED.name,
			character_set = EXCLUDED.character_set,
			"collation" = EXCLUDED.collation,
			schema_version = EXCLUDED.schema_version
		RETURNING id`
	var databaseUID int
	if err := tx.QueryRowContext(ctx, query,
		api.SystemBotID,
		api.SystemBotID,
		instance.UID,
		project.UID,
		create.DatabaseName,
		create.CharacterSet,
		create.Collation,
		api.OK,
		create.SuccessfulSyncTimeTs,
		create.SchemaVersion,
	).Scan(
		&databaseUID,
	); err != nil {
		return nil, FormatError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetDatabaseV2(ctx, &FindDatabaseMessage{UID: &databaseUID})
}

func (*Store) listDatabaseImplV2(ctx context.Context, tx *Tx, find *FindDatabaseMessage) ([]*DatabaseMessage, error) {
	where, args := []string{"1 = 1"}, []interface{}{}
	where, args = append(where, fmt.Sprintf("db.name != $%d", len(args)+1)), append(args, api.AllDatabaseName)
	if v := find.ProjectID; v != nil {
		where, args = append(where, fmt.Sprintf("project.resource_id = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.EnvironmentID; v != nil {
		where, args = append(where, fmt.Sprintf("environment.resource_id = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.InstanceID; v != nil {
		where, args = append(where, fmt.Sprintf("instance.resource_id = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.DatabaseName; v != nil {
		where, args = append(where, fmt.Sprintf("db.name = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.UID; v != nil {
		where, args = append(where, fmt.Sprintf("db.id = $%d", len(args)+1)), append(args, *v)
	}
	var databaseMessages []*DatabaseMessage
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			db.id,
			project.resource_id AS project_id,
			environment.resource_id AS environment_id,
			instance.resource_id AS instance_id,
			db.name,
			db.character_set,
			db.collation,
			db.sync_status,
			db.last_successful_sync_ts,
			db.schema_version,
			ARRAY_AGG (
				db_label.key
			) keys,
			ARRAY_AGG (
				db_label.value
			) values
		FROM db
		LEFT JOIN project ON db.project_id = project.id
		LEFT JOIN instance ON db.instance_id = instance.id
		LEFT JOIN environment ON instance.environment_id = environment.id
		LEFT JOIN db_label ON db.id = db_label.database_id
		WHERE %s
		GROUP BY db.id, project.resource_id, environment.resource_id, instance.resource_id`, strings.Join(where, " AND ")),
		args...,
	)
	if err != nil {
		return nil, FormatError(err)
	}
	defer rows.Close()
	for rows.Next() {
		databaseMessage := DatabaseMessage{
			Labels: make(map[string]string),
		}
		var keys, values []sql.NullString
		if err := rows.Scan(
			&databaseMessage.UID,
			&databaseMessage.ProjectID,
			&databaseMessage.EnvironmentID,
			&databaseMessage.InstanceID,
			&databaseMessage.DatabaseName,
			&databaseMessage.CharacterSet,
			&databaseMessage.Collation,
			&databaseMessage.SyncState,
			&databaseMessage.SuccessfulSyncTimeTs,
			&databaseMessage.SchemaVersion,
			pq.Array(&keys),
			pq.Array(&values),
		); err != nil {
			return nil, FormatError(err)
		}
		if len(keys) != len(values) {
			return nil, errors.Errorf("invalid length of database label keys and values")
		}
		for i := 0; i < len(keys); i++ {
			if !keys[i].Valid || !values[i].Valid {
				continue
			}
			databaseMessage.Labels[keys[i].String] = values[i].String
		}
		databaseMessages = append(databaseMessages, &databaseMessage)
	}

	return databaseMessages, nil
}
