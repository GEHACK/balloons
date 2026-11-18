use crate::domjudge::Config;
use bode_core::{balloon::GroupFilters, settings::Settings};
use sqlx::{SqlitePool, sqlite::SqliteConnectOptions};
use std::path::Path;

pub struct Store {
    pool: SqlitePool,
}

impl Store {
    pub async fn open(path: impl AsRef<Path>) -> Result<Store, sqlx::Error> {
        let opts = SqliteConnectOptions::new()
            .filename(path)
            .create_if_missing(true);
        let pool = SqlitePool::connect_with(opts).await?;
        sqlx::migrate!().run(&pool).await?;
        Ok(Store { pool })
    }

    pub async fn domjudge_config(&self) -> Result<Option<Config>, sqlx::Error> {
        sqlx::query_as!(
            Config,
            "SELECT base_url, user, password, contest_id FROM domjudge_connection"
        )
        .fetch_optional(&self.pool)
        .await
    }

    pub async fn set_domjudge_config(&self, config: &Config) -> Result<(), sqlx::Error> {
        sqlx::query!(
            "INSERT INTO domjudge_connection (id, base_url, user, password, contest_id)
             VALUES (1, ?, ?, ?, ?)
             ON CONFLICT (id) DO UPDATE SET
               base_url = excluded.base_url,
               user = excluded.user,
               password = excluded.password,
               contest_id = excluded.contest_id",
            config.base_url,
            config.user,
            config.password,
            config.contest_id,
        )
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn settings(&self) -> Result<Settings, sqlx::Error> {
        sqlx::query_as!(Settings, "SELECT timezone, loom_url FROM settings")
            .fetch_one(&self.pool)
            .await
    }

    pub async fn set_settings(&self, settings: &Settings) -> Result<(), sqlx::Error> {
        sqlx::query!(
            "INSERT INTO settings (id, timezone, loom_url)
             VALUES (1, ?, ?)
             ON CONFLICT (id) DO UPDATE SET
               timezone = excluded.timezone,
               loom_url = excluded.loom_url",
            settings.timezone,
            settings.loom_url,
        )
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn group_filters(&self) -> Result<GroupFilters, sqlx::Error> {
        let mut filters = GroupFilters::default();
        let rows = sqlx::query!("SELECT group_id, filter FROM group_filters")
            .fetch_all(&self.pool)
            .await?;

        for row in rows {
            match row.filter.as_str() {
                "hidden" => filters.hidden.insert(row.group_id),
                "no_first_solve" => filters.no_first_solve.insert(row.group_id),
                // Unknown filter names are ignored. TODO: log them once Bode has logging.
                _other => false,
            };
        }

        Ok(filters)
    }

    pub async fn set_group_filters(&self, new_filters: &GroupFilters) -> Result<(), sqlx::Error> {
        let mut tx = self.pool.begin().await?;

        sqlx::query!("DELETE FROM group_filters")
            .execute(&mut *tx)
            .await?;

        for group_id in &new_filters.no_first_solve {
            sqlx::query!(
                "INSERT INTO group_filters (group_id, filter) VALUES (?, 'no_first_solve')",
                group_id
            )
            .execute(&mut *tx)
            .await?;
        }

        for group_id in &new_filters.hidden {
            sqlx::query!(
                "INSERT INTO group_filters (group_id, filter) VALUES (?, 'hidden')",
                group_id
            )
            .execute(&mut *tx)
            .await?;
        }

        tx.commit().await?;
        Ok(())
    }
}
