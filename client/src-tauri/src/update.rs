#![cfg_attr(not(windows), allow(dead_code, unused_imports))]

//! Desktop update boundary. The webview may ask to check/apply an update, but
//! every URL, signature and executable path is revalidated in this module.

use crate::discovery::private_or_loopback_ipv4;
use base64::{engine::general_purpose::STANDARD as BASE64, Engine};
use minisign_verify::{PublicKey, Signature};
use reqwest::{redirect::Policy, Client, Url};
#[cfg(windows)]
use reqwest_updater::redirect::Policy as UpdaterRedirectPolicy;
use semver::Version;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::{
    fs,
    io::Read,
    net::Ipv4Addr,
    path::{Path, PathBuf},
    sync::Mutex,
    time::Duration,
};
use tauri::{AppHandle, Emitter, Manager, State};

const PLAN_PATH: &str = "/api/v1/updates/client/plan";
const READY_TIMEOUT: Duration = Duration::from_secs(90);
const PORTABLE_MARKER: &str = "bb-erp-portable.json";

#[derive(Default)]
pub struct UpdateEngine {
    checked_origin: Mutex<Option<String>>,
    snapshot: Mutex<UpdateSnapshot>,
    busy: Mutex<bool>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct UpdateSnapshot {
    pub state: String,
    pub message: Option<String>,
    pub downloaded_bytes: Option<u64>,
    pub total_bytes: Option<u64>,
    pub strategy: Option<String>,
}

impl Default for UpdateSnapshot {
    fn default() -> Self {
        Self {
            state: "Idle".into(),
            message: None,
            downloaded_bytes: None,
            total_bytes: None,
            strategy: None,
        }
    }
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct UpdateArtifact {
    pub kind: String,
    pub sha256: String,
    pub size: u64,
    pub signature: String,
    pub download_path: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ClientUpdatePlan {
    pub protocol_version: u32,
    pub current_version: String,
    pub latest_version: String,
    pub target: String,
    pub install_mode: String,
    pub strategy: String,
    pub download_size: u64,
    pub full_size: u64,
    pub signed_payload: String,
    pub signature: String,
    pub artifact: UpdateArtifact,
    #[serde(default)]
    pub message: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SignedPayload {
    protocol_version: u32,
    version: String,
    target: String,
    layout_version: u32,
    full: SignedFull,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SignedFull {
    nsis: SignedAsset,
    portable: SignedAsset,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SignedAsset {
    kind: String,
    #[serde(rename = "url")]
    _url: String,
    size: u64,
    sha256: String,
    signature: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct UpdateApplyResult {
    pub state: String,
    pub strategy: String,
    pub message: String,
}

pub fn update_public_key() -> Option<&'static str> {
    option_env!("BB_ERP_UPDATE_PUBLIC_KEY").filter(|key| !key.trim().is_empty())
}

fn error(message: impl Into<String>) -> String {
    message.into()
}

fn sha256_file(path: &Path) -> std::io::Result<String> {
    let mut input = fs::File::open(path)?;
    let mut hasher = Sha256::new();
    let mut buffer = [0_u8; 64 * 1024];
    loop {
        let count = input.read(&mut buffer)?;
        if count == 0 {
            break;
        }
        hasher.update(&buffer[..count]);
    }
    Ok(format!("{:x}", hasher.finalize()))
}

fn emit(engine: &UpdateEngine, app: &AppHandle, snapshot: UpdateSnapshot) {
    if let Ok(mut current) = engine.snapshot.lock() {
        *current = snapshot.clone();
    }
    let _ = app.emit("client-update-progress", snapshot);
}

fn begin_task(engine: &UpdateEngine) -> Result<(), String> {
    let mut busy = engine.busy.lock().map_err(|_| error("更新状态锁定失败"))?;
    if *busy {
        return Err(error("已有更新任务正在执行"));
    }
    *busy = true;
    Ok(())
}

fn finish_task(engine: &UpdateEngine) {
    if let Ok(mut busy) = engine.busy.lock() {
        *busy = false;
    }
}

fn clean_origin(value: &str) -> Result<Url, String> {
    let parsed = Url::parse(value.trim()).map_err(|_| error("服务器地址格式不正确"))?;
    let host = parsed
        .host_str()
        .ok_or_else(|| error("更新服务器地址缺少 IPv4 主机"))?;
    let ip: Ipv4Addr = host
        .parse()
        .map_err(|_| error("更新服务器必须使用本机或局域网 IPv4"))?;
    if parsed.scheme() != "http"
        || !private_or_loopback_ipv4(ip)
        || parsed.username() != ""
        || parsed.password().is_some()
        || parsed.path() != "/"
        || parsed.query().is_some()
        || parsed.fragment().is_some()
    {
        return Err(error("更新服务器地址必须是已验证的内网 HTTP 主机和端口"));
    }
    Ok(parsed)
}

fn same_origin(origin: &Url, url: &Url) -> bool {
    origin.scheme() == url.scheme()
        && origin.host_str() == url.host_str()
        && origin.port_or_known_default() == url.port_or_known_default()
}

fn artifact_url(origin: &Url, path: &str) -> Result<Url, String> {
    if !path.starts_with('/')
        || path.starts_with("//")
        || path.contains('\\')
        || path.chars().any(char::is_control)
    {
        return Err(error("更新资源路径不合法"));
    }
    let url = Url::parse(&format!(
        "{}{}",
        origin.origin().ascii_serialization(),
        path
    ))
    .map_err(|_| error("更新资源地址不合法"))?;
    if !same_origin(origin, &url) {
        return Err(error("更新资源必须来自已验证服务器"));
    }
    Ok(url)
}

fn http_client() -> Result<Client, String> {
    Client::builder()
        .no_proxy()
        .timeout(Duration::from_secs(20))
        .redirect(Policy::none())
        .build()
        .map_err(|e| e.to_string())
}

fn artifact_client() -> Result<Client, String> {
    Client::builder()
        .no_proxy()
        .timeout(Duration::from_secs(10 * 60))
        .redirect(Policy::none())
        .build()
        .map_err(|e| e.to_string())
}

fn current_version(app: &AppHandle) -> String {
    app.package_info().version.to_string()
}

async fn ensure_verified_server(origin: &Url) -> Result<(), String> {
    let health = artifact_url(origin, "/health")?;
    let response = http_client()?
        .get(health)
        .send()
        .await
        .map_err(|e| format!("无法验证服务器：{e}"))?;
    if !response.status().is_success() {
        return Err(error("服务器健康检查失败，拒绝下载更新"));
    }
    Ok(())
}

async fn fetch_plan(origin: &Url, app: &AppHandle) -> Result<Option<ClientUpdatePlan>, String> {
    let mut url = artifact_url(origin, PLAN_PATH)?;
    url.query_pairs_mut()
        .append_pair("current_version", &current_version(app))
        .append_pair("target", "windows-x86_64")
        .append_pair(
            "install_mode",
            if is_portable()? { "portable" } else { "nsis" },
        );
    let response = http_client()?
        .get(url)
        .send()
        .await
        .map_err(|e| format!("检查更新失败：{e}"))?;
    if response.status() == reqwest::StatusCode::NO_CONTENT {
        return Ok(None);
    }
    if !response.status().is_success() {
        return Err(format!("更新服务返回 HTTP {}", response.status()));
    }
    response
        .json::<ClientUpdatePlan>()
        .await
        .map(Some)
        .map_err(|e| format!("更新计划格式无效：{e}"))
}

fn verify_signature(bytes: &[u8], signature: &str) -> Result<(), String> {
    let public_key = update_public_key().ok_or_else(|| error("未配置更新公钥，已拒绝自动更新"))?;
    verify_signature_with_public_key(bytes, public_key, signature)
}

pub fn verify_signature_with_public_key(
    bytes: &[u8],
    public_key: &str,
    signature: &str,
) -> Result<(), String> {
    let key = parse_public_key(public_key)?;
    let signature = parse_signature_envelope(signature)?;
    key.verify(bytes, &signature, false)
        .map_err(|_| error("更新签名校验失败"))
}

/// Tauri's signer exposes the complete `.pub` file as a Base64 envelope.
pub fn parse_public_key(value: &str) -> Result<PublicKey, String> {
    let decoded = BASE64
        .decode(value.trim().as_bytes())
        .map_err(|_| error("更新公钥不是 Tauri Base64 封装"))?;
    let text = String::from_utf8(decoded).map_err(|_| error("更新公钥文本无效"))?;
    PublicKey::decode(&text).map_err(|_| error("更新公钥格式无效"))
}

fn decode_signature_envelope(value: &str) -> Result<String, String> {
    let decoded = BASE64
        .decode(value.trim().as_bytes())
        .map_err(|_| error("更新签名不是 Tauri Base64 封装"))?;
    String::from_utf8(decoded).map_err(|_| error("更新签名文本无效"))
}

pub fn parse_signature_envelope(value: &str) -> Result<Signature, String> {
    Signature::decode(&decode_signature_envelope(value)?).map_err(|_| error("更新签名格式无效"))
}

fn verify_plan(plan: &ClientUpdatePlan, origin: &Url, app: &AppHandle) -> Result<(), String> {
    if plan.protocol_version != 2 || plan.target != "windows-x86_64" {
        return Err(error("不支持的更新协议或平台"));
    }
    let current = current_version(app);
    let current_semver = Version::parse(current.trim_start_matches('v'))
        .map_err(|_| error("当前客户端版本号无效"))?;
    let latest_semver = Version::parse(plan.latest_version.trim_start_matches('v'))
        .map_err(|_| error("更新版本号无效"))?;
    if plan.current_version != current || latest_semver <= current_semver {
        return Err(error("更新版本号无效"));
    }
    let payload_bytes = BASE64
        .decode(plan.signed_payload.as_bytes())
        .map_err(|_| error("更新签名载荷不是 Base64"))?;
    verify_signature(&payload_bytes, &plan.signature)?;
    let payload: SignedPayload =
        serde_json::from_slice(&payload_bytes).map_err(|_| error("更新签名载荷格式无效"))?;
    if payload.protocol_version != 2
        || payload.layout_version != 1
        || payload.version != plan.latest_version
        || payload.target != plan.target
    {
        return Err(error("更新签名载荷与更新计划不一致"));
    }
    let expected_full = match plan.install_mode.as_str() {
        "portable" if plan.artifact.kind == "portable" => &payload.full.portable,
        "nsis" if plan.artifact.kind == "nsis" => &payload.full.nsis,
        _ => return Err(error("完整更新安装模式与资源类型不一致")),
    };
    if plan.strategy != "full"
        || plan.download_size != plan.artifact.size
        || plan.full_size != plan.artifact.size
    {
        return Err(error("更新计划不是当前 full-only 契约"));
    }
    verify_artifact_matches(&plan.artifact, expected_full, origin)?;
    Ok(())
}

fn verify_artifact_matches(
    actual: &UpdateArtifact,
    signed: &SignedAsset,
    origin: &Url,
) -> Result<(), String> {
    if actual.kind != signed.kind
        || actual.size != signed.size
        || !actual.sha256.eq_ignore_ascii_case(&signed.sha256)
        || actual.signature != signed.signature
    {
        return Err(error("更新资源与签名载荷不一致"));
    }
    let actual_url = artifact_url(origin, &actual.download_path)?;
    if !same_origin(origin, &actual_url) {
        return Err(error("更新下载路径必须来自已验证的内网服务"));
    }
    Ok(())
}

fn cache_path(app: &AppHandle, sha: &str) -> Result<PathBuf, String> {
    if sha.len() != 64 || !sha.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err(error("更新资源哈希不合法"));
    }
    let directory = app
        .path()
        .app_data_dir()
        .map_err(|e| e.to_string())?
        .join("updates");
    fs::create_dir_all(&directory).map_err(|e| e.to_string())?;
    Ok(directory.join(sha.to_ascii_lowercase()))
}

async fn download_verified(
    app: &AppHandle,
    engine: &UpdateEngine,
    origin: &Url,
    artifact: &UpdateArtifact,
) -> Result<PathBuf, String> {
    let target = cache_path(app, &artifact.sha256)?;
    if target.is_file()
        && fs::metadata(&target).map_err(|e| e.to_string())?.len() == artifact.size
        && sha256_file(&target)
            .map_err(|e| e.to_string())?
            .eq_ignore_ascii_case(&artifact.sha256)
    {
        return Ok(target);
    }
    let temporary = target.with_extension("part");
    let url = artifact_url(origin, &artifact.download_path)?;
    let response = artifact_client()?
        .get(url)
        .send()
        .await
        .map_err(|e| format!("下载更新失败：{e}"))?;
    if !response.status().is_success() {
        return Err(format!("下载更新返回 HTTP {}", response.status()));
    }
    let public_key = update_public_key().ok_or_else(|| error("未配置更新公钥，已拒绝自动更新"))?;
    let key = parse_public_key(public_key)?;
    let signature = parse_signature_envelope(&artifact.signature)?;
    let mut verifier = key
        .verify_stream(&signature)
        .map_err(|_| error("更新资源必须使用预哈希 Minisign 签名"))?;
    let mut file = fs::File::create(&temporary).map_err(|e| e.to_string())?;
    let mut hasher = Sha256::new();
    let mut downloaded = 0_u64;
    let mut response = response;
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|e| format!("读取更新失败：{e}"))?
    {
        downloaded += chunk.len() as u64;
        if downloaded > artifact.size {
            let _ = fs::remove_file(&temporary);
            return Err(error("更新资源大小校验失败"));
        }
        use std::io::Write;
        file.write_all(&chunk).map_err(|e| e.to_string())?;
        hasher.update(&chunk);
        verifier.update(&chunk);
        emit(
            engine,
            app,
            UpdateSnapshot {
                state: "Downloading".into(),
                message: Some("正在下载更新".into()),
                downloaded_bytes: Some(downloaded),
                total_bytes: Some(artifact.size),
                strategy: Some("full".into()),
            },
        );
    }
    file.sync_all().map_err(|e| e.to_string())?;
    if downloaded != artifact.size {
        let _ = fs::remove_file(&temporary);
        return Err(error("更新资源大小校验失败"));
    }
    let digest = format!("{:x}", hasher.finalize());
    if !digest.eq_ignore_ascii_case(&artifact.sha256) {
        let _ = fs::remove_file(&temporary);
        return Err(error("更新资源 SHA-256 校验失败"));
    }
    verifier.finalize().map_err(|_| {
        let _ = fs::remove_file(&temporary);
        error("更新资源签名校验失败")
    })?;
    fs::rename(&temporary, &target).map_err(|e| e.to_string())?;
    emit(
        engine,
        app,
        UpdateSnapshot {
            state: "Verifying".into(),
            message: Some("更新资源已通过签名与哈希校验".into()),
            downloaded_bytes: Some(downloaded),
            total_bytes: Some(artifact.size),
            strategy: Some("full".into()),
        },
    );
    Ok(target)
}

fn is_portable() -> Result<bool, String> {
    let exe = std::env::current_exe().map_err(|e| e.to_string())?;
    Ok(exe
        .parent()
        .is_some_and(|directory| directory.join(PORTABLE_MARKER).is_file()))
}

#[tauri::command]
pub async fn client_update_check(
    app: AppHandle,
    engine: State<'_, UpdateEngine>,
    server_url: String,
) -> Result<Option<ClientUpdatePlan>, String> {
    #[cfg(not(windows))]
    {
        let _ = (app, engine, server_url);
        return Err(error("当前平台不支持 Windows 客户端自动更新"));
    }
    #[cfg(windows)]
    {
        begin_task(&engine)?;
        let result: Result<Option<ClientUpdatePlan>, String> = async {
            let origin = clean_origin(&server_url)?;
            emit(
                &engine,
                &app,
                UpdateSnapshot {
                    state: "Checking".into(),
                    message: Some("正在验证更新服务器".into()),
                    ..Default::default()
                },
            );
            ensure_verified_server(&origin).await?;
            let plan = fetch_plan(&origin, &app).await?;
            if let Some(ref plan) = plan {
                verify_plan(plan, &origin, &app)?;
            }
            *engine
                .checked_origin
                .lock()
                .map_err(|_| error("更新状态锁定失败"))? =
                Some(origin.origin().ascii_serialization());
            emit(
                &engine,
                &app,
                UpdateSnapshot {
                    state: if plan.is_some() { "Ready" } else { "Idle" }.into(),
                    message: Some(
                        if plan.is_some() {
                            "发现可用更新"
                        } else {
                            "当前已是最新版本"
                        }
                        .into(),
                    ),
                    ..Default::default()
                },
            );
            Ok(plan)
        }
        .await;
        finish_task(&engine);
        if let Err(ref message) = result {
            emit(
                &engine,
                &app,
                UpdateSnapshot {
                    state: "Failed".into(),
                    message: Some(message.clone()),
                    ..Default::default()
                },
            );
        }
        result
    }
}

#[tauri::command]
pub fn client_update_status(engine: State<'_, UpdateEngine>) -> UpdateSnapshot {
    engine
        .snapshot
        .lock()
        .map(|state| state.clone())
        .unwrap_or_default()
}

#[tauri::command]
pub async fn client_update_apply(
    app: AppHandle,
    engine: State<'_, UpdateEngine>,
    plan: ClientUpdatePlan,
) -> Result<UpdateApplyResult, String> {
    #[cfg(not(windows))]
    {
        let _ = (app, engine, plan);
        return Err(error("当前平台不支持 Windows 客户端自动更新"));
    }
    #[cfg(windows)]
    {
        begin_task(&engine)?;
        let result: Result<UpdateApplyResult, String> = async {
            let origin = engine
                .checked_origin
                .lock()
                .map_err(|_| error("更新状态锁定失败"))?
                .clone()
                .ok_or_else(|| error("请先通过已验证服务器检查更新"))?;
            let origin = clean_origin(&origin)?;
            // Never trust the webview-provided plan. Fetch the latest one from the server again.
            let current = fetch_plan(&origin, &app)
                .await?
                .ok_or_else(|| error("更新已不可用，请重新检查"))?;
            if current.latest_version != plan.latest_version
                || current.artifact.sha256 != plan.artifact.sha256
            {
                return Err(error("更新计划已变化，请重新检查"));
            }
            verify_plan(&current, &origin, &app)?;
            emit(
                &engine,
                &app,
                UpdateSnapshot {
                    state: "Downloading".into(),
                    message: Some("正在下载更新".into()),
                    strategy: Some(current.strategy.clone()),
                    ..Default::default()
                },
            );
            let message = apply_full(&app, &engine, &origin, &current).await?;
            Ok(UpdateApplyResult {
                state: "Restarting".into(),
                strategy: "full".into(),
                message,
            })
        }
        .await;
        finish_task(&engine);
        if let Err(ref message) = result {
            emit(
                &engine,
                &app,
                UpdateSnapshot {
                    state: "Failed".into(),
                    message: Some(message.clone()),
                    ..Default::default()
                },
            );
        }
        result
    }
}

#[cfg(windows)]
async fn apply_full(
    app: &AppHandle,
    engine: &UpdateEngine,
    origin: &Url,
    plan: &ClientUpdatePlan,
) -> Result<String, String> {
    let current = std::env::current_exe().map_err(|e| e.to_string())?;
    if should_use_portable_full(
        is_portable()?,
        ensure_target_parent_writable(&current).is_ok(),
    ) {
        let replacement = download_verified(app, engine, origin, &plan.artifact).await?;
        emit(
            engine,
            app,
            UpdateSnapshot {
                state: "Applying".into(),
                message: Some("正在准备完整客户端替换".into()),
                strategy: Some("full".into()),
                ..Default::default()
            },
        );
        return schedule_portable_replace(app, engine, &replacement);
    }
    apply_nsis_full(app, engine, origin, plan).await
}

fn should_use_portable_full(portable_marker: bool, target_writable: bool) -> bool {
    portable_marker && target_writable
}

#[cfg(windows)]
async fn apply_nsis_full(
    app: &AppHandle,
    engine: &UpdateEngine,
    origin: &Url,
    plan: &ClientUpdatePlan,
) -> Result<String, String> {
    use tauri_plugin_updater::UpdaterExt;
    let endpoint = artifact_url(
        origin,
        &format!(
            "/api/v1/updates/client/tauri/windows/x86_64/{}",
            current_version(app)
        ),
    )?;
    let updater = app
        .updater_builder()
        .no_proxy()
        .configure_client(|builder| builder.redirect(UpdaterRedirectPolicy::none()))
        .endpoints(vec![endpoint])
        .map_err(|e| e.to_string())?
        .build()
        .map_err(|e| e.to_string())?;
    let update = updater
        .check()
        .await
        .map_err(|e| format!("完整更新检查失败：{e}"))?
        .ok_or_else(|| error("完整更新已不可用"))?;
    validate_nsis_update_metadata(
        &plan.latest_version,
        &plan.artifact,
        origin,
        &update.version,
        &update.download_url,
        &update.signature,
    )?;
    let mut total_downloaded = 0_u64;
    emit(
        engine,
        app,
        UpdateSnapshot {
            state: "Downloading".into(),
            message: Some("正在下载完整更新".into()),
            strategy: Some("full".into()),
            ..Default::default()
        },
    );
    update
        .download_and_install(
            |chunk_size, total| {
                total_downloaded += chunk_size as u64;
                emit(
                    engine,
                    app,
                    UpdateSnapshot {
                        state: "Downloading".into(),
                        message: Some("正在下载完整更新".into()),
                        downloaded_bytes: Some(total_downloaded),
                        total_bytes: total,
                        strategy: Some("full".into()),
                    },
                )
            },
            || {
                emit(
                    engine,
                    app,
                    UpdateSnapshot {
                        state: "Applying".into(),
                        message: Some("正在验证并安装完整更新".into()),
                        strategy: Some("full".into()),
                        ..Default::default()
                    },
                )
            },
        )
        .await
        .map_err(|e| format!("完整更新安装失败：{e}"))?;
    emit(
        engine,
        app,
        UpdateSnapshot {
            state: "Restarting".into(),
            message: Some("完整更新安装完成，正在重启客户端".into()),
            strategy: Some("full".into()),
            ..Default::default()
        },
    );
    Ok("完整安装程序已启动，客户端将重启".into())
}

fn validate_nsis_update_metadata(
    planned_version: &str,
    artifact: &UpdateArtifact,
    origin: &Url,
    update_version: &str,
    download_url: &Url,
    signature: &str,
) -> Result<(), String> {
    let expected_url = artifact_url(origin, &artifact.download_path)?;
    if artifact.kind != "nsis"
        || update_version != planned_version
        || download_url != &expected_url
        || signature != artifact.signature
    {
        return Err(error("安装器更新信息与已验签计划不一致"));
    }
    Ok(())
}

#[cfg(windows)]
fn schedule_portable_replace(
    app: &AppHandle,
    engine: &UpdateEngine,
    replacement: &Path,
) -> Result<String, String> {
    use std::{
        process::Command,
        time::{SystemTime, UNIX_EPOCH},
    };
    let current = std::env::current_exe().map_err(|e| e.to_string())?;
    ensure_target_parent_writable(&current)?;
    let cache = app
        .path()
        .app_data_dir()
        .map_err(|e| e.to_string())?
        .join("updates");
    fs::create_dir_all(&cache).map_err(|e| e.to_string())?;
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|e| e.to_string())?
        .as_nanos();
    let helper = cache.join(format!("apply-{nonce}.exe"));
    let ready = cache.join(format!("ready-{nonce}.marker"));
    let staged_replacement = stage_replacement_for_target(replacement, &current)?;
    fs::copy(&current, &helper).map_err(|e| {
        let _ = fs::remove_file(&staged_replacement);
        format!("无法创建更新助手：{e}")
    })?;
    let pid = std::process::id();
    Command::new(&helper)
        .args([
            "--apply-client-update",
            "--target",
            &current.to_string_lossy(),
            "--replacement",
            &staged_replacement.to_string_lossy(),
            "--parent-pid",
            &pid.to_string(),
            "--ready-marker",
            &ready.to_string_lossy(),
        ])
        .spawn()
        .map_err(|e| {
            let _ = fs::remove_file(&staged_replacement);
            let _ = fs::remove_file(&helper);
            format!("无法启动更新助手：{e}")
        })?;
    emit(
        engine,
        app,
        UpdateSnapshot {
            state: "Restarting".into(),
            message: Some("更新已就绪，正在重启客户端".into()),
            strategy: Some("full".into()),
            ..Default::default()
        },
    );
    app.exit(0);
    Ok("更新助手已启动，客户端正在重启".into())
}

/// Called by `main` before Tauri starts. It only accepts paths generated by
/// `schedule_portable_replace`; no webview command can invoke this mode.
#[cfg(windows)]
pub fn try_run_update_helper() -> bool {
    let args: Vec<String> = std::env::args().collect();
    if !args.iter().any(|arg| arg == "--apply-client-update") {
        return false;
    }
    if let Err(error) = run_update_helper(&args) {
        eprintln!("bb-erp updater: {error}");
    }
    true
}

#[cfg(not(windows))]
pub fn try_run_update_helper() -> bool {
    false
}

#[cfg(windows)]
fn helper_value(args: &[String], name: &str) -> Result<PathBuf, String> {
    let index = args
        .iter()
        .position(|arg| arg == name)
        .ok_or_else(|| error("更新助手参数不完整"))?;
    args.get(index + 1)
        .map(PathBuf::from)
        .ok_or_else(|| error("更新助手参数不完整"))
}

#[cfg(windows)]
fn run_update_helper(args: &[String]) -> Result<(), String> {
    use std::{process::Command, thread, time::Instant};
    let target = helper_value(args, "--target")?;
    let replacement = helper_value(args, "--replacement")?;
    let ready = helper_value(args, "--ready-marker")?;
    let parent_pid = args
        .iter()
        .position(|arg| arg == "--parent-pid")
        .and_then(|i| args.get(i + 1))
        .ok_or_else(|| error("更新助手参数不完整"))?
        .parse::<u32>()
        .map_err(|_| error("更新助手 PID 不合法"))?;
    if !target.is_absolute()
        || !replacement.is_absolute()
        || !ready.is_absolute()
        || !replacement.is_file()
    {
        return Err(error("更新助手路径不合法"));
    }
    if replacement.parent() != target.parent() {
        return Err(error("更新暂存文件必须位于客户端安装目录"));
    }
    let backup = target.with_extension("old");
    let old_exit_deadline = phase_deadline(Instant::now());
    while process_exists(parent_pid) && Instant::now() < old_exit_deadline {
        thread::sleep(Duration::from_millis(250));
    }
    if process_exists(parent_pid) {
        let _ = fs::remove_file(&replacement);
        return Err(error("等待旧客户端退出超时"));
    }
    let _ = fs::remove_file(&ready);
    let _ = fs::remove_file(&backup);
    if let Err(error) = fs::rename(&target, &backup) {
        let _ = fs::remove_file(&replacement);
        let _ = restart_original(&target);
        return Err(format!("无法备份旧客户端：{error}"));
    }
    if let Err(e) = fs::rename(&replacement, &target) {
        let _ = fs::remove_file(&replacement);
        let _ = fs::rename(&backup, &target);
        let _ = restart_original(&target);
        return Err(format!("无法替换客户端：{e}"));
    }
    let mut child = match Command::new(&target)
        .arg("--update-ready-marker")
        .arg(&ready)
        .spawn()
    {
        Ok(child) => child,
        Err(spawn_error) => {
            let recovery = recover_original(&target, &backup, restart_original);
            return Err(combine_update_errors(
                "无法启动新客户端",
                spawn_error.to_string(),
                recovery,
            ));
        }
    };
    let ready_deadline = phase_deadline(Instant::now());
    let mut exited_early = false;
    while !ready.is_file() && Instant::now() < ready_deadline {
        if child
            .try_wait()
            .map_err(|e| format!("无法检查新客户端状态：{e}"))?
            .is_some()
        {
            exited_early = true;
            break;
        }
        thread::sleep(Duration::from_millis(250));
    }
    if ready.is_file() {
        let _ = fs::remove_file(&backup);
        let _ = fs::remove_file(&ready);
        return Ok(());
    }
    if !exited_early {
        let _ = child.kill();
    }
    let _ = child.wait();
    let recovery = recover_original(&target, &backup, restart_original);
    let reason = if exited_early {
        "新客户端提前退出"
    } else {
        "新客户端启动超时"
    };
    match recovery {
        Ok(()) => Err(format!("{reason}，已恢复旧版本")),
        Err(recovery_error) => Err(format!("{reason}，且恢复旧版本失败：{recovery_error}")),
    }
}

fn phase_deadline(now: std::time::Instant) -> std::time::Instant {
    now + READY_TIMEOUT
}

/// Restore the backed-up executable before reporting an update failure. The
/// restart is intentionally best effort, but both replacement and restart
/// errors are retained for the administrator.
fn recover_original<F>(target: &Path, backup: &Path, restart: F) -> Result<(), String>
where
    F: FnOnce(&Path) -> Result<(), String>,
{
    let mut failures = Vec::new();
    if let Err(error) = fs::remove_file(target) {
        if error.kind() != std::io::ErrorKind::NotFound {
            failures.push(format!("删除新客户端失败：{error}"));
        }
    }
    if let Err(error) = fs::rename(backup, target) {
        failures.push(format!("恢复旧客户端失败：{error}"));
    }
    if let Err(error) = restart(target) {
        failures.push(error);
    }
    if failures.is_empty() {
        Ok(())
    } else {
        Err(failures.join("；"))
    }
}

fn combine_update_errors(prefix: &str, primary: String, recovery: Result<(), String>) -> String {
    match recovery {
        Ok(()) => format!("{prefix}：{primary}；已恢复旧版本"),
        Err(recovery_error) => format!("{prefix}：{primary}；恢复旧版本失败：{recovery_error}"),
    }
}

#[cfg(windows)]
fn restart_original(target: &Path) -> Result<(), String> {
    std::process::Command::new(target)
        .spawn()
        .map(|_| ())
        .map_err(|e| format!("无法恢复旧客户端：{e}"))
}

/// Copy the verified cache artifact into the target executable's directory
/// before the parent exits. This makes the helper's final rename same-volume
/// and therefore atomic on Windows even when AppData is on another drive.
fn stage_replacement_for_target(replacement: &Path, target: &Path) -> Result<PathBuf, String> {
    use std::{
        fs::OpenOptions,
        io::{copy, Write},
    };
    let directory = target.parent().ok_or_else(|| error("客户端路径不合法"))?;
    ensure_target_parent_writable(target)?;
    let mut random = [0_u8; 16];
    getrandom::fill(&mut random).map_err(|_| error("无法生成更新暂存文件名"))?;
    let target_name = target
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("client.exe");
    let staged = directory.join(format!(".{target_name}.{}.staging", hex_name(&random)));
    let mut source = fs::File::open(replacement).map_err(|e| format!("无法读取已验证更新：{e}"))?;
    let mut destination = OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&staged)
        .map_err(|e| format!("无法创建同目录更新暂存文件：{e}"))?;
    let result = copy(&mut source, &mut destination)
        .and_then(|_| destination.flush())
        .and_then(|_| destination.sync_all());
    if let Err(error) = result {
        let _ = fs::remove_file(&staged);
        return Err(format!("无法写入更新暂存文件：{error}"));
    }
    let hashes = sha256_file(replacement)
        .map_err(|e| e.to_string())
        .and_then(|source_hash| {
            sha256_file(&staged)
                .map(|staged_hash| (source_hash, staged_hash))
                .map_err(|e| e.to_string())
        });
    let Ok((source_hash, staged_hash)) = hashes else {
        let _ = fs::remove_file(&staged);
        return Err(error("更新暂存文件哈希校验失败"));
    };
    if !source_hash.eq_ignore_ascii_case(&staged_hash) {
        let _ = fs::remove_file(&staged);
        return Err(error("更新暂存文件哈希校验失败"));
    }
    Ok(staged)
}

fn hex_name(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}

/// Probe the target directory before the old process exits. Windows directory
/// ACLs are authoritative for rename/create operations; Program Files installs
/// use the signed NSIS installer instead of direct executable replacement.
fn ensure_target_parent_writable(target: &Path) -> Result<(), String> {
    use std::{
        fs::OpenOptions,
        io::Write,
        time::{SystemTime, UNIX_EPOCH},
    };
    let directory = target.parent().ok_or_else(|| error("客户端路径不合法"))?;
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|e| e.to_string())?
        .as_nanos();
    let probe = directory.join(format!(".bb-erp-update-{nonce}.probe"));
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&probe)
        .map_err(|_| error("客户端安装目录不可写，将改用完整安装更新"))?;
    file.write_all(b"probe")
        .and_then(|_| file.sync_all())
        .map_err(|_| error("客户端安装目录不可写，将改用完整安装更新"))?;
    fs::remove_file(&probe).map_err(|_| error("客户端安装目录不可写，将改用完整安装更新"))?;
    Ok(())
}

#[cfg(windows)]
fn process_exists(pid: u32) -> bool {
    use std::process::Command;
    Command::new("tasklist")
        .args(["/FI", &format!("PID eq {pid}"), "/NH"])
        .output()
        .map(|output| String::from_utf8_lossy(&output.stdout).contains(&pid.to_string()))
        .unwrap_or(false)
}

pub fn mark_ready_from_args() {
    let args: Vec<String> = std::env::args().collect();
    if let Some(index) = args.iter().position(|arg| arg == "--update-ready-marker") {
        if let Some(marker) = args.get(index + 1) {
            let _ = fs::write(marker, b"ready");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn server_origin_rejects_paths_and_credentials() {
        assert!(clean_origin("http://192.168.1.2:8080").is_ok());
        assert!(clean_origin("http://127.0.0.1:8080").is_ok());
        assert!(clean_origin("http://8.8.8.8:8080").is_err());
        assert!(clean_origin("http://updates.example.test").is_err());
        assert!(clean_origin("https://192.168.1.2:8080").is_err());
        assert!(clean_origin("http://user:pass@localhost:8080").is_err());
        assert!(clean_origin("http://localhost:8080/api").is_err());
        assert!(clean_origin("file:///tmp/a").is_err());
    }

    #[test]
    fn artifact_paths_cannot_escape_the_verified_origin() {
        let origin = clean_origin("http://192.168.1.2:8080").unwrap();
        assert!(artifact_url(&origin, "/api/v1/updates/client/artifacts/abc").is_ok());
        assert!(artifact_url(&origin, "https://attacker.test/a").is_err());
        assert!(artifact_url(&origin, "//attacker.test/a").is_err());
        assert!(artifact_url(&origin, "/\\attacker.test/a").is_err());
    }

    #[test]
    fn nsis_download_must_match_the_verified_plan() {
        let origin = clean_origin("http://192.168.1.2:8080").unwrap();
        let artifact = UpdateArtifact {
            kind: "nsis".into(),
            sha256: "a".repeat(64),
            size: 1,
            signature: "signed-installer".into(),
            download_path: "/api/v1/updates/client/artifacts/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa".into(),
        };
        let expected_url = artifact_url(&origin, &artifact.download_path).unwrap();
        assert!(validate_nsis_update_metadata(
            "1.0.1",
            &artifact,
            &origin,
            "1.0.1",
            &expected_url,
            "signed-installer",
        )
        .is_ok());
        assert!(validate_nsis_update_metadata(
            "1.0.1",
            &artifact,
            &origin,
            "1.0.0",
            &expected_url,
            "signed-installer",
        )
        .is_err());
        assert!(validate_nsis_update_metadata(
            "1.0.1",
            &artifact,
            &origin,
            "1.0.1",
            &Url::parse("http://192.168.1.3:8080/installer.exe").unwrap(),
            "signed-installer",
        )
        .is_err());
        assert!(validate_nsis_update_metadata(
            "1.0.1",
            &artifact,
            &origin,
            "1.0.1",
            &expected_url,
            "different-signature",
        )
        .is_err());
    }

    #[test]
    fn full_only_contract_rejects_delta_and_legacy_plan_fields() {
        let current = br#"{
          "protocol_version":2,
          "version":"1.0.1",
          "target":"windows-x86_64",
          "layout_version":1,
          "full":{
            "nsis":{"kind":"nsis","url":"http://192.168.1.2/nsis.exe","size":1,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","signature":"sig"},
            "portable":{"kind":"portable","url":"http://192.168.1.2/client.exe","size":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","signature":"sig"}
          }
        }"#;
        assert!(serde_json::from_slice::<SignedPayload>(current).is_ok());

        let with_delta = br#"{
          "protocol_version":2,
          "version":"1.0.1",
          "target":"windows-x86_64",
          "layout_version":1,
          "full":{
            "nsis":{"kind":"nsis","url":"http://192.168.1.2/nsis.exe","size":1,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","signature":"sig"},
            "portable":{"kind":"portable","url":"http://192.168.1.2/client.exe","size":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","signature":"sig"}
          },
          "deltas":[]
        }"#;
        assert!(serde_json::from_slice::<SignedPayload>(with_delta).is_err());

        let plan = br#"{
          "protocol_version":2,
          "current_version":"1.0.0",
          "latest_version":"1.0.1",
          "target":"windows-x86_64",
          "install_mode":"portable",
          "strategy":"full",
          "download_size":1,
          "full_size":1,
          "saved_bytes":0,
          "signed_payload":"payload",
          "signature":"signature",
          "artifact":{"kind":"portable","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":1,"signature":"sig","download_path":"/api/v1/updates/client/artifacts/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
        }"#;
        assert!(serde_json::from_slice::<ClientUpdatePlan>(plan).is_err());
    }

    #[test]
    fn decodes_tauri_signature_transport_envelope() {
        let source =
            "untrusted comment: signature\nRkFLRQ==\ntrusted comment: timestamp:0\nRkFLRQ==";
        let encoded = BASE64.encode(source.as_bytes());
        assert_eq!(decode_signature_envelope(&encoded).unwrap(), source);
        assert!(decode_signature_envelope("not base64!!").is_err());
        let invalid_text = BASE64.encode(b"this is not a minisign signature");
        assert!(parse_signature_envelope(&invalid_text).is_err());
    }

    #[test]
    fn rejects_a_tampered_tauri_signature_or_payload() {
        let signature = "untrusted comment: signature from minisign secret key\nRUQf6LRCGA9i559r3g7V1qNyJDApGip8MfqcadIgT9CuhV3EMhHoN1mGTkUidF/z7SrlQgXdy8ofjb7bNJJylDOocrCo8KLzZwo=\ntrusted comment: timestamp:1556193335\tfile:test\ny/rUw2y8/hOUYjZU71eHp/Wo1KZ40fGy2VJEDl34XMJM+TX48Ss/17u3IvIfbVR1FkZZSNCisQbuQY+bHwhEBg==";
        let envelope = BASE64.encode(signature.as_bytes());
        let raw_key = "RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3";
        let public_file =
            format!("untrusted comment: minisign public key E7620F1842B4E81F\n{raw_key}");
        let key = BASE64.encode(public_file.as_bytes());
        verify_signature_with_public_key(b"test", &key, &envelope).unwrap();
        assert!(verify_signature_with_public_key(b"Test", &key, &envelope).is_err());
        let tampered = BASE64.encode(signature.replacen("RUQf", "RUQe", 1).as_bytes());
        assert!(verify_signature_with_public_key(b"test", &key, &tampered).is_err());
    }

    #[test]
    fn accepts_only_the_current_tauri_public_key_envelope() {
        let raw = "RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3";
        let public_file = format!("untrusted comment: minisign public key E7620F1842B4E81F\n{raw}");
        let envelope = BASE64.encode(public_file.as_bytes());
        assert!(parse_public_key(&envelope).is_ok());
        assert!(parse_public_key(raw).is_err());
        assert!(parse_public_key("this is not a public key").is_err());
    }

    #[test]
    fn busy_guard_rejects_a_second_update_task() {
        let engine = UpdateEngine::default();
        begin_task(&engine).unwrap();
        assert!(begin_task(&engine).is_err());
        finish_task(&engine);
        assert!(begin_task(&engine).is_ok());
    }

    #[test]
    fn portable_full_requires_a_writable_target() {
        assert!(should_use_portable_full(true, true));
        assert!(!should_use_portable_full(true, false));
        assert!(!should_use_portable_full(false, true));
    }

    #[test]
    fn writable_probe_accepts_a_writable_target_directory() {
        let directory =
            std::env::temp_dir().join(format!("bb-erp-update-probe-{}", std::process::id()));
        fs::create_dir_all(&directory).unwrap();
        let target = directory.join("client.exe");
        ensure_target_parent_writable(&target).unwrap();
        let _ = fs::remove_dir(directory);
    }

    #[test]
    fn recovery_restores_backup_when_new_client_cannot_start() {
        use std::sync::{
            atomic::{AtomicBool, Ordering},
            Arc,
        };
        let directory =
            std::env::temp_dir().join(format!("bb-erp-recovery-{}", std::process::id()));
        fs::create_dir_all(&directory).unwrap();
        let target = directory.join("client.exe");
        let backup = directory.join("client.old");
        fs::write(&target, b"new-client").unwrap();
        fs::write(&backup, b"old-client").unwrap();
        let restarted = Arc::new(AtomicBool::new(false));
        let restarted_for_callback = restarted.clone();
        recover_original(&target, &backup, move |path| {
            assert_eq!(fs::read(path).unwrap(), b"old-client");
            restarted_for_callback.store(true, Ordering::SeqCst);
            Ok(())
        })
        .unwrap();
        assert_eq!(fs::read(&target).unwrap(), b"old-client");
        assert!(!backup.exists());
        assert!(restarted.load(Ordering::SeqCst));
        let _ = fs::remove_file(target);
        let _ = fs::remove_dir(directory);
    }

    #[test]
    fn update_phases_receive_independent_deadlines() {
        let old_exit_started = std::time::Instant::now();
        let old_exit_deadline = phase_deadline(old_exit_started);
        let ready_started = old_exit_deadline;
        let ready_deadline = phase_deadline(ready_started);
        assert_eq!(
            old_exit_deadline.duration_since(old_exit_started),
            READY_TIMEOUT
        );
        assert_eq!(ready_deadline.duration_since(ready_started), READY_TIMEOUT);
        assert!(ready_deadline > old_exit_deadline);
    }

    #[test]
    fn staging_places_cross_directory_artifact_next_to_target() {
        let root =
            std::env::temp_dir().join(format!("bb-erp-staging-cross-{}", std::process::id()));
        let cache = root.join("cache");
        let install = root.join("install");
        fs::create_dir_all(&cache).unwrap();
        fs::create_dir_all(&install).unwrap();
        let replacement = cache.join("replacement.exe");
        let target = install.join("client.exe");
        fs::write(&replacement, b"verified replacement from another volume").unwrap();
        let staged = stage_replacement_for_target(&replacement, &target).unwrap();
        assert_eq!(staged.parent(), target.parent());
        assert_ne!(staged.parent(), replacement.parent());
        assert_eq!(
            sha256_file(&staged).unwrap(),
            sha256_file(&replacement).unwrap()
        );
        let _ = fs::remove_file(staged);
        let _ = fs::remove_file(replacement);
        let _ = fs::remove_dir(cache);
        let _ = fs::remove_dir(install);
        let _ = fs::remove_dir(root);
    }

    #[test]
    fn staging_uses_a_distinct_file_in_the_target_directory() {
        let root =
            std::env::temp_dir().join(format!("bb-erp-staging-local-{}", std::process::id()));
        fs::create_dir_all(&root).unwrap();
        let replacement = root.join("downloaded.exe");
        let target = root.join("client.exe");
        fs::write(&replacement, b"verified replacement in same directory").unwrap();
        let staged = stage_replacement_for_target(&replacement, &target).unwrap();
        assert_eq!(staged.parent(), target.parent());
        assert_ne!(staged, replacement);
        assert!(staged
            .file_name()
            .unwrap()
            .to_string_lossy()
            .ends_with(".staging"));
        assert_eq!(
            fs::read(&replacement).unwrap(),
            b"verified replacement in same directory"
        );
        let _ = fs::remove_file(staged);
        let _ = fs::remove_file(replacement);
        let _ = fs::remove_dir(root);
    }
}
