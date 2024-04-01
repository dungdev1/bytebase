/* eslint-disable */

export const protobufPackage = "bytebase.v1";

export enum State {
  STATE_UNSPECIFIED = 0,
  ACTIVE = 1,
  DELETED = 2,
  UNRECOGNIZED = -1,
}

export function stateFromJSON(object: any): State {
  switch (object) {
    case 0:
    case "STATE_UNSPECIFIED":
      return State.STATE_UNSPECIFIED;
    case 1:
    case "ACTIVE":
      return State.ACTIVE;
    case 2:
    case "DELETED":
      return State.DELETED;
    case -1:
    case "UNRECOGNIZED":
    default:
      return State.UNRECOGNIZED;
  }
}

export function stateToJSON(object: State): string {
  switch (object) {
    case State.STATE_UNSPECIFIED:
      return "STATE_UNSPECIFIED";
    case State.ACTIVE:
      return "ACTIVE";
    case State.DELETED:
      return "DELETED";
    case State.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

export enum Engine {
  ENGINE_UNSPECIFIED = 0,
  CLICKHOUSE = 1,
  MYSQL = 2,
  POSTGRES = 3,
  SNOWFLAKE = 4,
  SQLITE = 5,
  TIDB = 6,
  MONGODB = 7,
  REDIS = 8,
  ORACLE = 9,
  SPANNER = 10,
  MSSQL = 11,
  REDSHIFT = 12,
  MARIADB = 13,
  OCEANBASE = 14,
  DM = 15,
  RISINGWAVE = 16,
  OCEANBASE_ORACLE = 17,
  STARROCKS = 18,
  DORIS = 19,
  HIVE = 20,
  UNRECOGNIZED = -1,
}

export function engineFromJSON(object: any): Engine {
  switch (object) {
    case 0:
    case "ENGINE_UNSPECIFIED":
      return Engine.ENGINE_UNSPECIFIED;
    case 1:
    case "CLICKHOUSE":
      return Engine.CLICKHOUSE;
    case 2:
    case "MYSQL":
      return Engine.MYSQL;
    case 3:
    case "POSTGRES":
      return Engine.POSTGRES;
    case 4:
    case "SNOWFLAKE":
      return Engine.SNOWFLAKE;
    case 5:
    case "SQLITE":
      return Engine.SQLITE;
    case 6:
    case "TIDB":
      return Engine.TIDB;
    case 7:
    case "MONGODB":
      return Engine.MONGODB;
    case 8:
    case "REDIS":
      return Engine.REDIS;
    case 9:
    case "ORACLE":
      return Engine.ORACLE;
    case 10:
    case "SPANNER":
      return Engine.SPANNER;
    case 11:
    case "MSSQL":
      return Engine.MSSQL;
    case 12:
    case "REDSHIFT":
      return Engine.REDSHIFT;
    case 13:
    case "MARIADB":
      return Engine.MARIADB;
    case 14:
    case "OCEANBASE":
      return Engine.OCEANBASE;
    case 15:
    case "DM":
      return Engine.DM;
    case 16:
    case "RISINGWAVE":
      return Engine.RISINGWAVE;
    case 17:
    case "OCEANBASE_ORACLE":
      return Engine.OCEANBASE_ORACLE;
    case 18:
    case "STARROCKS":
      return Engine.STARROCKS;
    case 19:
    case "DORIS":
      return Engine.DORIS;
    case 20:
    case "HIVE":
      return Engine.HIVE;
    case -1:
    case "UNRECOGNIZED":
    default:
      return Engine.UNRECOGNIZED;
  }
}

export function engineToJSON(object: Engine): string {
  switch (object) {
    case Engine.ENGINE_UNSPECIFIED:
      return "ENGINE_UNSPECIFIED";
    case Engine.CLICKHOUSE:
      return "CLICKHOUSE";
    case Engine.MYSQL:
      return "MYSQL";
    case Engine.POSTGRES:
      return "POSTGRES";
    case Engine.SNOWFLAKE:
      return "SNOWFLAKE";
    case Engine.SQLITE:
      return "SQLITE";
    case Engine.TIDB:
      return "TIDB";
    case Engine.MONGODB:
      return "MONGODB";
    case Engine.REDIS:
      return "REDIS";
    case Engine.ORACLE:
      return "ORACLE";
    case Engine.SPANNER:
      return "SPANNER";
    case Engine.MSSQL:
      return "MSSQL";
    case Engine.REDSHIFT:
      return "REDSHIFT";
    case Engine.MARIADB:
      return "MARIADB";
    case Engine.OCEANBASE:
      return "OCEANBASE";
    case Engine.DM:
      return "DM";
    case Engine.RISINGWAVE:
      return "RISINGWAVE";
    case Engine.OCEANBASE_ORACLE:
      return "OCEANBASE_ORACLE";
    case Engine.STARROCKS:
      return "STARROCKS";
    case Engine.DORIS:
      return "DORIS";
    case Engine.HIVE:
      return "HIVE";
    case Engine.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

export enum VCSType {
  VCS_TYPE_UNSPECIFIED = 0,
  /** GITHUB - GitHub type. Using for GitHub community edition(ce). */
  GITHUB = 1,
  /** GITLAB - GitLab type. Using for GitLab community edition(ce) and enterprise edition(ee). */
  GITLAB = 2,
  /** BITBUCKET - BitBucket type. Using for BitBucket cloud or BitBucket server. */
  BITBUCKET = 3,
  /** AZURE_DEVOPS - Azure DevOps. Using for Azure DevOps GitOps workflow. */
  AZURE_DEVOPS = 4,
  UNRECOGNIZED = -1,
}

export function vCSTypeFromJSON(object: any): VCSType {
  switch (object) {
    case 0:
    case "VCS_TYPE_UNSPECIFIED":
      return VCSType.VCS_TYPE_UNSPECIFIED;
    case 1:
    case "GITHUB":
      return VCSType.GITHUB;
    case 2:
    case "GITLAB":
      return VCSType.GITLAB;
    case 3:
    case "BITBUCKET":
      return VCSType.BITBUCKET;
    case 4:
    case "AZURE_DEVOPS":
      return VCSType.AZURE_DEVOPS;
    case -1:
    case "UNRECOGNIZED":
    default:
      return VCSType.UNRECOGNIZED;
  }
}

export function vCSTypeToJSON(object: VCSType): string {
  switch (object) {
    case VCSType.VCS_TYPE_UNSPECIFIED:
      return "VCS_TYPE_UNSPECIFIED";
    case VCSType.GITHUB:
      return "GITHUB";
    case VCSType.GITLAB:
      return "GITLAB";
    case VCSType.BITBUCKET:
      return "BITBUCKET";
    case VCSType.AZURE_DEVOPS:
      return "AZURE_DEVOPS";
    case VCSType.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

export enum MaskingLevel {
  MASKING_LEVEL_UNSPECIFIED = 0,
  NONE = 1,
  PARTIAL = 2,
  FULL = 3,
  UNRECOGNIZED = -1,
}

export function maskingLevelFromJSON(object: any): MaskingLevel {
  switch (object) {
    case 0:
    case "MASKING_LEVEL_UNSPECIFIED":
      return MaskingLevel.MASKING_LEVEL_UNSPECIFIED;
    case 1:
    case "NONE":
      return MaskingLevel.NONE;
    case 2:
    case "PARTIAL":
      return MaskingLevel.PARTIAL;
    case 3:
    case "FULL":
      return MaskingLevel.FULL;
    case -1:
    case "UNRECOGNIZED":
    default:
      return MaskingLevel.UNRECOGNIZED;
  }
}

export function maskingLevelToJSON(object: MaskingLevel): string {
  switch (object) {
    case MaskingLevel.MASKING_LEVEL_UNSPECIFIED:
      return "MASKING_LEVEL_UNSPECIFIED";
    case MaskingLevel.NONE:
      return "NONE";
    case MaskingLevel.PARTIAL:
      return "PARTIAL";
    case MaskingLevel.FULL:
      return "FULL";
    case MaskingLevel.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

export enum ExportFormat {
  FORMAT_UNSPECIFIED = 0,
  CSV = 1,
  JSON = 2,
  SQL = 3,
  XLSX = 4,
  UNRECOGNIZED = -1,
}

export function exportFormatFromJSON(object: any): ExportFormat {
  switch (object) {
    case 0:
    case "FORMAT_UNSPECIFIED":
      return ExportFormat.FORMAT_UNSPECIFIED;
    case 1:
    case "CSV":
      return ExportFormat.CSV;
    case 2:
    case "JSON":
      return ExportFormat.JSON;
    case 3:
    case "SQL":
      return ExportFormat.SQL;
    case 4:
    case "XLSX":
      return ExportFormat.XLSX;
    case -1:
    case "UNRECOGNIZED":
    default:
      return ExportFormat.UNRECOGNIZED;
  }
}

export function exportFormatToJSON(object: ExportFormat): string {
  switch (object) {
    case ExportFormat.FORMAT_UNSPECIFIED:
      return "FORMAT_UNSPECIFIED";
    case ExportFormat.CSV:
      return "CSV";
    case ExportFormat.JSON:
      return "JSON";
    case ExportFormat.SQL:
      return "SQL";
    case ExportFormat.XLSX:
      return "XLSX";
    case ExportFormat.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}
