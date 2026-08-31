//! Private-network ERP service discovery and connection verification.
//!
//! The webview never chooses an arbitrary host: UDP only supplies a candidate
//! IP/port and this module verifies the ready and identity endpoints directly.

use getrandom::fill as random_fill;
use if_addrs::{get_if_addrs, IfAddr};
use reqwest::{redirect::Policy, Client, StatusCode, Url};
use serde::{Deserialize, Serialize};
use std::{
    collections::BTreeMap,
    net::{Ipv4Addr, SocketAddr, UdpSocket},
    time::{Duration, Instant},
};
use uuid::Uuid;

const DISCOVERY_PORT: u16 = 39080;
const DISCOVERY_PROTOCOL: u32 = 1;
const PRODUCT: &str = "bb-erp";
const MAX_RESPONSE_BYTES: usize = 16 * 1024;
const DISCOVERY_WINDOW: Duration = Duration::from_millis(2500);
const MAX_CANDIDATES: usize = 24;
const VERIFY_CONCURRENCY: usize = 4;
const VERIFY_DEADLINE: Duration = Duration::from_secs(6);

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct ServerIdentity {
    pub product: String,
    pub discovery_protocol: u32,
    pub instance_id: String,
    pub server_name: String,
    pub server_version: String,
    #[serde(default)]
    pub origin: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct Announcement {
    kind: String,
    protocol: u32,
    nonce: String,
    product: String,
    instance_id: String,
    server_name: String,
    http_port: u16,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ReadyResponse {
    status: String,
}

pub fn private_or_loopback_ipv4(ip: Ipv4Addr) -> bool {
    let [a, b, ..] = ip.octets();
    a == 127 || a == 10 || (a == 172 && (16..=31).contains(&b)) || (a == 192 && b == 168)
}

pub(crate) fn internal_http_origin(value: &str) -> Result<Url, String> {
    let parsed = Url::parse(value.trim()).map_err(|_| "服务器地址格式不正确".to_string())?;
    let host = parsed
        .host_str()
        .ok_or_else(|| "服务器地址缺少 IPv4 主机".to_string())?;
    let ip: Ipv4Addr = host
        .parse()
        .map_err(|_| "服务器地址必须是本机或局域网 IPv4".to_string())?;
    if parsed.scheme() != "http"
        || !private_or_loopback_ipv4(ip)
        || parsed.username() != ""
        || parsed.password().is_some()
        || parsed.path() != "/"
        || parsed.query().is_some()
        || parsed.fragment().is_some()
    {
        return Err("服务器地址只能是本机或局域网 IPv4 的 HTTP 主机和端口".to_string());
    }
    Ok(parsed)
}

fn origin_for(ip: Ipv4Addr, port: u16) -> Result<Url, String> {
    if !private_or_loopback_ipv4(ip) || port == 0 {
        return Err("发现响应包含不安全的地址或端口".to_string());
    }
    Url::parse(&format!("http://{ip}:{port}")).map_err(|_| "无法构造服务地址".to_string())
}

fn nonce() -> Result<String, String> {
    let mut bytes = [0u8; 16];
    random_fill(&mut bytes).map_err(|e| format!("无法生成发现随机数：{e}"))?;
    Ok(bytes.iter().map(|byte| format!("{byte:02x}")).collect())
}

fn parse_announcement(bytes: &[u8], expected_nonce: &str) -> Option<Announcement> {
    if bytes.is_empty() || bytes.len() > 512 {
        return None;
    }
    let value: Announcement = serde_json::from_slice(bytes).ok()?;
    (value.kind == "announce"
        && value.protocol == DISCOVERY_PROTOCOL
        && value.product == PRODUCT
        && value.nonce == expected_nonce
        && Uuid::parse_str(&value.instance_id).is_ok()
        && valid_display_text(&value.server_name, 120, false)
        && value.http_port != 0)
        .then_some(value)
}

fn valid_display_text(value: &str, max_chars: usize, allow_empty: bool) -> bool {
    let trimmed = value.trim();
    (allow_empty || !trimmed.is_empty())
        && trimmed.chars().count() <= max_chars
        && !trimmed.chars().any(char::is_control)
}

fn http_client() -> Result<Client, String> {
    Client::builder()
        .no_proxy()
        .redirect(Policy::none())
        .connect_timeout(Duration::from_millis(1500))
        .timeout(Duration::from_secs(4))
        .build()
        .map_err(|e| format!("无法初始化本地网络验证：{e}"))
}

async fn json_limited<T: serde::de::DeserializeOwned>(
    response: reqwest::Response,
) -> Result<T, String> {
    if response.status() != StatusCode::OK {
        return Err(format!("服务返回 HTTP {}", response.status()));
    }
    if response
        .content_length()
        .is_some_and(|length| length > MAX_RESPONSE_BYTES as u64)
    {
        return Err("服务验证响应过大".to_string());
    }
    let mut response = response;
    let mut body = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|e| format!("读取服务响应失败：{e}"))?
    {
        if body.len() + chunk.len() > MAX_RESPONSE_BYTES {
            return Err("服务验证响应过大".to_string());
        }
        body.extend_from_slice(&chunk);
    }
    serde_json::from_slice(&body).map_err(|_| "服务验证响应格式不正确".to_string())
}

async fn verify_origin(
    origin: Url,
    expected: Option<&Announcement>,
) -> Result<ServerIdentity, String> {
    let client = http_client()?;
    let ready_url = origin
        .join("/ready")
        .map_err(|_| "服务地址无效".to_string())?;
    let ready: ReadyResponse = json_limited(
        client
            .get(ready_url)
            .send()
            .await
            .map_err(|e| format!("无法连接服务：{e}"))?,
    )
    .await?;
    if ready.status != "ready" {
        return Err("服务尚未完成数据库初始化".to_string());
    }
    let identity_url = origin
        .join("/api/v1/discovery/identity")
        .map_err(|_| "服务地址无效".to_string())?;
    let mut identity: ServerIdentity = json_limited(
        client
            .get(identity_url)
            .send()
            .await
            .map_err(|e| format!("无法读取服务身份：{e}"))?,
    )
    .await?;
    if identity.product != PRODUCT
        || identity.discovery_protocol != DISCOVERY_PROTOCOL
        || Uuid::parse_str(&identity.instance_id).is_err()
        || !valid_display_text(&identity.server_name, 120, false)
        || !valid_display_text(&identity.server_version, 64, true)
    {
        return Err("目标服务不是受支持的博邦 ERP 服务".to_string());
    }
    if let Some(announcement) = expected {
        if identity.instance_id != announcement.instance_id
            || identity.server_name != announcement.server_name
            || identity.product != announcement.product
            || identity.discovery_protocol != announcement.protocol
        {
            return Err("发现响应与服务身份不一致".to_string());
        }
    }
    identity.origin = origin.origin().ascii_serialization();
    Ok(identity)
}

pub(crate) async fn verify_internal_origin(origin: Url) -> Result<ServerIdentity, String> {
    verify_origin(origin, None).await
}

#[tauri::command]
pub async fn test_server_connection(server_url: String) -> Result<ServerIdentity, String> {
    verify_internal_origin(internal_http_origin(&server_url)?).await
}

#[tauri::command]
pub async fn discover_servers() -> Result<Vec<ServerIdentity>, String> {
    let candidates = tauri::async_runtime::spawn_blocking(discover_candidates)
        .await
        .map_err(|e| format!("局域网发现任务失败：{e}"))??;
    let mut identities = BTreeMap::new();
    let deadline = Instant::now() + VERIFY_DEADLINE;
    for batch in candidates.chunks(VERIFY_CONCURRENCY) {
        if Instant::now() >= deadline {
            break;
        }
        let mut tasks = Vec::with_capacity(batch.len());
        for (origin, announcement) in batch.iter().cloned() {
            tasks.push(tauri::async_runtime::spawn(async move {
                verify_origin(origin, Some(&announcement)).await
            }));
        }
        let remaining = deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            break;
        }
        let completed = tokio::time::timeout(remaining, async {
            let mut verified = Vec::with_capacity(tasks.len());
            for task in &mut tasks {
                if let Ok(Ok(identity)) = task.await {
                    verified.push(identity);
                }
            }
            verified
        })
        .await;
        let Ok(verified) = completed else {
            for task in tasks {
                task.abort();
            }
            break;
        };
        for identity in verified {
            let key = format!("{}|{}", identity.origin, identity.instance_id);
            identities.entry(key).or_insert(identity);
        }
    }
    Ok(identities.into_values().collect())
}

fn discover_candidates() -> Result<Vec<(Url, Announcement)>, String> {
    let nonce = nonce()?;
    let payload =
        serde_json::json!({"kind": "discover", "protocol": DISCOVERY_PROTOCOL, "nonce": nonce})
            .to_string();
    let socket = UdpSocket::bind((Ipv4Addr::UNSPECIFIED, 0))
        .map_err(|e| format!("无法启动局域网发现：{e}"))?;
    socket
        .set_broadcast(true)
        .map_err(|e| format!("无法启用局域网广播：{e}"))?;
    let targets = private_broadcast_targets()?;
    let mut sent = 0usize;
    for target in targets {
        if socket.send_to(payload.as_bytes(), target).is_ok() {
            sent += 1;
        }
    }
    if sent == 0 {
        return Err("没有可用的局域网 IPv4 网卡，无法发送发现广播".to_string());
    }
    socket
        .set_read_timeout(Some(Duration::from_millis(150)))
        .map_err(|e| format!("无法设置发现超时：{e}"))?;
    let deadline = Instant::now() + DISCOVERY_WINDOW;
    let mut candidates = BTreeMap::new();
    let mut buffer = [0u8; 2048];
    while Instant::now() < deadline {
        match socket.recv_from(&mut buffer) {
            Ok((length, SocketAddr::V4(source))) => {
                if private_or_loopback_ipv4(*source.ip()) {
                    if let Some(announcement) = parse_announcement(&buffer[..length], &nonce) {
                        if let Ok(origin) = origin_for(*source.ip(), announcement.http_port) {
                            let key = format!(
                                "{}:{}:{}",
                                source.ip(),
                                announcement.http_port,
                                announcement.instance_id
                            );
                            if candidates.len() < MAX_CANDIDATES {
                                candidates.entry(key).or_insert((origin, announcement));
                            }
                        }
                    }
                }
            }
            Ok(_) => {}
            Err(error)
                if error.kind() == std::io::ErrorKind::WouldBlock
                    || error.kind() == std::io::ErrorKind::TimedOut => {}
            Err(error) => return Err(format!("读取局域网发现响应失败：{error}")),
        }
    }
    Ok(candidates.into_values().collect())
}

fn private_broadcast_targets() -> Result<Vec<SocketAddr>, String> {
    let interfaces = get_if_addrs().map_err(|e| format!("无法读取局域网网卡：{e}"))?;
    let mut targets = BTreeMap::new();
    for interface in interfaces {
        let IfAddr::V4(address) = interface.addr else {
            continue;
        };
        if address.ip.is_loopback() || !private_or_loopback_ipv4(address.ip) {
            continue;
        }
        let ip = u32::from(address.ip);
        let mask = u32::from(address.netmask);
        let broadcast = Ipv4Addr::from(ip | !mask);
        targets.insert(broadcast, SocketAddr::from((broadcast, DISCOVERY_PORT)));
    }
    Ok(targets.into_values().collect())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::{
        io::{Read, Write},
        net::TcpListener,
        thread,
    };

    #[test]
    fn accepts_only_private_or_loopback_ipv4() {
        assert!(private_or_loopback_ipv4(Ipv4Addr::new(127, 8, 0, 1)));
        assert!(private_or_loopback_ipv4(Ipv4Addr::new(10, 0, 0, 1)));
        assert!(private_or_loopback_ipv4(Ipv4Addr::new(172, 31, 1, 1)));
        assert!(private_or_loopback_ipv4(Ipv4Addr::new(192, 168, 1, 1)));
        assert!(!private_or_loopback_ipv4(Ipv4Addr::new(172, 32, 1, 1)));
        assert!(!private_or_loopback_ipv4(Ipv4Addr::new(8, 8, 8, 8)));
    }

    #[test]
    fn internal_origin_rejects_hostname_https_and_paths() {
        assert!(internal_http_origin("http://192.168.1.3:8080").is_ok());
        assert!(internal_http_origin("https://192.168.1.3:8080").is_err());
        assert!(internal_http_origin("http://erp.local:8080").is_err());
        assert!(internal_http_origin("http://192.168.1.3:8080/api").is_err());
    }

    #[test]
    fn nonce_is_lowercase_128_bit_hex() {
        let value = nonce().unwrap();
        assert_eq!(value.len(), 32);
        assert!(value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte)));
    }

    #[test]
    fn announcement_requires_matching_protocol_product_and_nonce() {
        let payload = br#"{"kind":"announce","protocol":1,"nonce":"good","product":"bb-erp","instance_id":"550e8400-e29b-41d4-a716-446655440000","server_name":"name","http_port":8080}"#;
        assert!(parse_announcement(payload, "good").is_some());
        assert!(parse_announcement(payload, "wrong").is_none());
    }

    #[test]
    fn same_instance_id_on_different_origins_remains_a_conflict() {
        let mut values = BTreeMap::new();
        for origin in ["http://10.0.0.1:8080", "http://10.0.0.2:8080"] {
            values.entry(format!("{origin}|same")).or_insert(origin);
        }
        assert_eq!(values.len(), 2);
    }

    #[test]
    fn identity_rejects_unknown_fields_and_unsafe_display_text() {
        assert!(!valid_display_text("ERP\nServer", 120, false));
        assert!(!valid_display_text("", 120, false));
        assert!(valid_display_text("ERP Server", 120, false));

        let payload = br#"{"product":"bb-erp","discovery_protocol":1,"instance_id":"550e8400-e29b-41d4-a716-446655440000","server_name":"ERP","server_version":"1.0.0","unexpected":true}"#;
        assert!(serde_json::from_slice::<ServerIdentity>(payload).is_err());
    }

    #[test]
    fn verification_requires_ready_and_returns_verified_identity() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        let port = listener.local_addr().unwrap().port();
        let server = thread::spawn(move || {
            for body in [
                r#"{"status":"ready"}"#,
                r#"{"product":"bb-erp","discovery_protocol":1,"instance_id":"550e8400-e29b-41d4-a716-446655440000","server_name":"ERP","server_version":"1.0.0"}"#,
            ] {
                let (mut stream, _) = listener.accept().unwrap();
                let mut request = [0u8; 512];
                let _ = stream.read(&mut request).unwrap();
                write!(
                    stream,
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body.len(), body
                )
                .unwrap();
                stream.flush().unwrap();
            }
        });
        let origin = origin_for(Ipv4Addr::LOCALHOST, port).unwrap();
        let identity = tauri::async_runtime::block_on(verify_origin(origin, None)).unwrap();
        server.join().unwrap();
        assert_eq!(identity.instance_id, "550e8400-e29b-41d4-a716-446655440000");
        assert_eq!(identity.origin, format!("http://127.0.0.1:{port}"));
    }
}
