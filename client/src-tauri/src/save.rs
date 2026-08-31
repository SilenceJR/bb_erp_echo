//! Native save boundary for protected ERP downloads.
//!
//! The webview supplies only a same-origin API path.  Rust revalidates the
//! current private-network server, obtains an explicit user-selected target,
//! then streams into a sibling temporary file before replacing the target.

use crate::discovery::{internal_http_origin, verify_internal_origin};
use getrandom::fill as random_fill;
use reqwest::{header::HeaderValue, redirect::Policy, Client};
use serde::Serialize;
use std::{
    ffi::OsStr,
    fs::{self, OpenOptions},
    io::Write,
    path::{Path, PathBuf},
    time::Duration,
};
use tauri::AppHandle;
use tauri_plugin_dialog::DialogExt;

const MAX_ERROR_BYTES: usize = 4096;

#[derive(Clone, Debug, Serialize, PartialEq, Eq)]
#[serde(tag = "status", rename_all = "snake_case")]
pub enum FileSaveResult {
    Saved { path: String },
    Cancelled,
    Error { message: String },
}

fn safe_file_name(value: &str) -> Result<&str, String> {
    let name = value.trim();
    if name.is_empty()
        || name == "."
        || name == ".."
        || Path::new(name).file_name() != Some(OsStr::new(name))
        || name.contains('\\')
        || name.chars().any(char::is_control)
    {
        return Err("保存文件名无效".to_string());
    }
    Ok(name)
}

fn api_url(server_url: &str, path: &str) -> Result<reqwest::Url, String> {
    if !path.starts_with('/')
        || path.starts_with("//")
        || path.contains('\\')
        || path.chars().any(char::is_control)
    {
        return Err("下载地址必须是当前服务的站内 API 路径".to_string());
    }
    let origin = internal_http_origin(server_url)?;
    let joined = origin.join(path).map_err(|_| "下载地址无效".to_string())?;
    if joined.origin() != origin.origin()
        || joined.scheme() != origin.scheme()
        || joined.host_str() != origin.host_str()
        || joined.port_or_known_default() != origin.port_or_known_default()
    {
        return Err("下载地址不能离开当前已验证服务器".to_string());
    }
    Ok(joined)
}

fn save_client() -> Result<Client, String> {
    Client::builder()
        .no_proxy()
        .redirect(Policy::none())
        .connect_timeout(Duration::from_secs(3))
        .timeout(Duration::from_secs(15 * 60))
        .build()
        .map_err(|error| format!("无法初始化本地文件下载：{error}"))
}

fn temporary_path(target: &Path) -> Result<PathBuf, String> {
    let parent = target.parent().ok_or_else(|| "保存位置无效".to_string())?;
    let stem = target
        .file_name()
        .and_then(OsStr::to_str)
        .ok_or_else(|| "保存文件名无效".to_string())?;
    for _ in 0..8 {
        let mut bytes = [0u8; 16];
        random_fill(&mut bytes).map_err(|error| format!("无法创建临时文件：{error}"))?;
        let suffix: String = bytes.iter().map(|byte| format!("{byte:02x}")).collect();
        let candidate = parent.join(format!(".{stem}.{suffix}.part"));
        if !candidate.exists() {
            return Ok(candidate);
        }
    }
    Err("无法创建唯一临时文件".to_string())
}

#[cfg(windows)]
fn atomic_replace(source: &Path, target: &Path) -> Result<(), String> {
    use std::os::windows::ffi::OsStrExt;

    extern "system" {
        fn MoveFileExW(existing: *const u16, new: *const u16, flags: u32) -> i32;
    }
    const MOVEFILE_REPLACE_EXISTING: u32 = 0x1;
    const MOVEFILE_WRITE_THROUGH: u32 = 0x8;
    let source: Vec<u16> = source.as_os_str().encode_wide().chain(Some(0)).collect();
    let target: Vec<u16> = target.as_os_str().encode_wide().chain(Some(0)).collect();
    // The vectors remain alive for the call and both paths are local dialog-selected paths.
    if unsafe {
        MoveFileExW(
            source.as_ptr(),
            target.as_ptr(),
            MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH,
        )
    } == 0
    {
        return Err(format!(
            "无法完成文件保存：{}",
            std::io::Error::last_os_error()
        ));
    }
    Ok(())
}

#[cfg(not(windows))]
fn atomic_replace(source: &Path, target: &Path) -> Result<(), String> {
    fs::rename(source, target).map_err(|error| format!("无法完成文件保存：{error}"))
}

async fn error_body(response: reqwest::Response) -> String {
    let status = response.status();
    let mut response = response;
    let mut body = Vec::new();
    while let Ok(Some(chunk)) = response.chunk().await {
        let remaining = MAX_ERROR_BYTES.saturating_sub(body.len());
        if remaining == 0 {
            break;
        }
        body.extend_from_slice(&chunk[..chunk.len().min(remaining)]);
    }
    let detail = String::from_utf8_lossy(&body).trim().to_string();
    if detail.is_empty() {
        format!("服务返回 HTTP {status}")
    } else {
        format!("服务返回 HTTP {status}：{detail}")
    }
}

async fn save_inner(
    app: &AppHandle,
    server_url: &str,
    api_path: &str,
    file_name: &str,
    token: &str,
) -> Result<FileSaveResult, String> {
    let file_name = safe_file_name(file_name)?;
    let origin = internal_http_origin(server_url)?;
    // A download is accepted only from a live ERP server at the exact current
    // origin. This also prevents stale/manual origins from bypassing connection validation.
    verify_internal_origin(origin).await?;
    let Some(target) = app
        .dialog()
        .file()
        .set_file_name(file_name)
        .blocking_save_file()
    else {
        return Ok(FileSaveResult::Cancelled);
    };
    let target = target
        .into_path()
        .map_err(|_| "保存位置必须是本地文件路径".to_string())?;
    let url = api_url(server_url, api_path)?;
    let client = save_client()?;
    let mut request = client.get(url).header("Accept", "application/octet-stream");
    if !token.trim().is_empty() {
        let header = HeaderValue::from_str(&format!("Bearer {token}"))
            .map_err(|_| "下载凭据格式无效".to_string())?;
        request = request.header("Authorization", header);
    }
    let mut response = request
        .send()
        .await
        .map_err(|error| format!("下载文件失败：{error}"))?;
    if !response.status().is_success() {
        return Err(error_body(response).await);
    }
    let temporary = temporary_path(&target)?;
    let result = async {
        let mut file = OpenOptions::new()
            .create_new(true)
            .write(true)
            .open(&temporary)
            .map_err(|error| format!("无法创建临时文件：{error}"))?;
        while let Some(chunk) = response
            .chunk()
            .await
            .map_err(|error| format!("读取下载内容失败：{error}"))?
        {
            file.write_all(&chunk)
                .map_err(|error| format!("写入文件失败：{error}"))?;
        }
        file.sync_all()
            .map_err(|error| format!("写入文件失败：{error}"))?;
        drop(file);
        atomic_replace(&temporary, &target)?;
        Ok::<_, String>(())
    }
    .await;
    if let Err(error) = result {
        let _ = fs::remove_file(&temporary);
        return Err(error);
    }
    Ok(FileSaveResult::Saved {
        path: target.to_string_lossy().into_owned(),
    })
}

#[tauri::command]
pub async fn save_api_file(
    app: AppHandle,
    server_url: String,
    api_path: String,
    file_name: String,
    token: String,
) -> FileSaveResult {
    match save_inner(&app, &server_url, &api_path, &file_name, &token).await {
        Ok(result) => result,
        Err(message) => FileSaveResult::Error { message },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn file_name_rejects_paths_and_controls() {
        assert_eq!(safe_file_name("客户资料.xlsx"), Ok("客户资料.xlsx"));
        assert!(safe_file_name("../secret.xlsx").is_err());
        assert!(safe_file_name("a/b.xlsx").is_err());
        assert!(safe_file_name("a\\b.xlsx").is_err());
        assert!(safe_file_name("bad\nname.xlsx").is_err());
    }

    #[test]
    fn api_url_stays_on_private_server_origin() {
        assert!(api_url("http://192.168.1.2:8080", "/api/v1/files/1").is_ok());
        assert!(api_url("http://192.168.1.2:8080", "https://invalid.example/file").is_err());
        assert!(api_url("http://8.8.8.8:8080", "/api/v1/files/1").is_err());
        assert!(api_url("http://192.168.1.2:8080", "/\\8.8.8.8:8080/steal").is_err());
        assert!(api_url("http://192.168.1.2:8080", "/api/v1/files/1\nnext").is_err());
    }

    #[test]
    fn temporary_file_is_sibling_of_target() {
        let target = std::env::temp_dir().join("bb-erp-save-test.xlsx");
        let temporary = temporary_path(&target).unwrap();
        assert_eq!(temporary.parent(), target.parent());
        assert!(temporary
            .extension()
            .is_some_and(|extension| extension == "part"));
    }
}
