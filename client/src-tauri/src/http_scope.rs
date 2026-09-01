use std::str::FromStr;

use tauri_utils::acl::RemoteUrlPattern;

const CAPABILITY_JSON: &str = include_str!("../capabilities/default.json");

fn http_allow_patterns() -> Vec<String> {
    let capabilities: serde_json::Value =
        serde_json::from_str(CAPABILITY_JSON).expect("默认 capability 配置必须是有效 JSON");
    capabilities["permissions"]
        .as_array()
        .expect("默认 capability 必须包含权限列表")
        .iter()
        .find(|permission| permission["identifier"] == "http:default")
        .and_then(|permission| permission["allow"].as_array())
        .expect("http:default 必须声明 allow 范围")
        .iter()
        .map(|entry| {
            entry["url"]
                .as_str()
                .expect("HTTP allow 项必须包含 URL Pattern")
                .to_owned()
        })
        .collect()
}

fn url_is_allowed(url: &str) -> bool {
    let request_url = url.parse().expect("测试 URL 必须有效");
    http_allow_patterns().iter().any(|pattern| {
        RemoteUrlPattern::from_str(pattern)
            .expect("HTTP allow URL Pattern 必须有效")
            .test(&request_url)
    })
}

#[test]
fn private_ipv4_http_scope_allows_supported_ranges() {
    for url in [
        "http://127.0.0.1:8080/health",
        "http://10.2.3.4:8080/api/v1/auth/login",
        "http://172.16.0.1:8080/api/v1/auth/login",
        "http://172.31.255.255:8080/api/v1/auth/login",
        "http://192.168.124.83:8080/api/v1/auth/login",
    ] {
        assert!(url_is_allowed(url), "应授权私网 HTTP URL：{url}");
    }
}

#[test]
fn private_ipv4_http_scope_rejects_unsupported_origins() {
    for url in [
        "http://172.32.0.1:8080/api/v1/auth/login",
        "http://192.169.1.1:8080/api/v1/auth/login",
        "http://8.8.8.8:8080/api/v1/auth/login",
        "https://192.168.124.83:8080/api/v1/auth/login",
    ] {
        assert!(!url_is_allowed(url), "不应授权 URL：{url}");
    }
}
