use bode_backend::store::Store;
use tempfile::{TempDir, tempdir};

/// A store on an empty database in a temporary directory. Keep the `TempDir`
/// alive for the whole test: dropping it deletes the database.
pub async fn fresh_store() -> (Store, TempDir) {
    let tempdir = tempdir().expect("Making tempdir failed");
    let store = Store::open(tempdir.path().join("db.db"))
        .await
        .expect("Opening db failed");
    (store, tempdir)
}
