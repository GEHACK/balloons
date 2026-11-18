use reqwest::Method;
use serde::de::DeserializeOwned;
use std::time::Duration;

#[derive(Debug, Clone, PartialEq)]
pub struct Config {
    pub base_url: String,
    pub user: String,
    pub password: String,
    pub contest_id: String,
}

pub struct Client {
    http: reqwest::Client,
    config: Config,
}

impl Client {
    pub fn new(config: Config) -> Result<Self, reqwest::Error> {
        let http = reqwest::Client::builder()
            .timeout(Duration::from_secs(5))
            .build()?;
        Ok(Client { http, config })
    }

    fn url(&self, path: &str) -> String {
        format!(
            "{}/api/v4/contests/{}/{}",
            self.config.base_url.trim_end_matches('/'),
            self.config.contest_id,
            path
        )
    }

    fn request(&self, method: Method, path: &str) -> reqwest::RequestBuilder {
        self.http
            .request(method, self.url(path))
            .basic_auth(&self.config.user, Some(&self.config.password))
    }

    async fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T, reqwest::Error> {
        self.request(Method::GET, path)
            .send()
            .await?
            .error_for_status()?
            .json()
            .await
    }

    pub async fn fetch_balloons(
        &self,
    ) -> Result<Vec<bode_core::domjudge::Balloon>, reqwest::Error> {
        self.get("balloons").await
    }

    pub async fn fetch_teams(&self) -> Result<Vec<bode_core::domjudge::Team>, reqwest::Error> {
        self.get("teams").await
    }

    pub async fn fetch_problems(
        &self,
    ) -> Result<Vec<bode_core::domjudge::ContestProblem>, reqwest::Error> {
        self.get("problems").await
    }

    pub async fn fetch_contest_state(
        &self,
    ) -> Result<bode_core::domjudge::ContestState, reqwest::Error> {
        self.get("state").await
    }

    pub async fn mark_balloon_delivered(&self, balloon_id: u32) -> Result<(), reqwest::Error> {
        self.request(Method::POST, &format!("balloons/{balloon_id}/done"))
            .send()
            .await?
            .error_for_status()?;
        Ok(())
    }
}
