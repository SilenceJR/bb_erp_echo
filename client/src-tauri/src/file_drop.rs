use reqwest::multipart::Form;
use serde::{Deserialize, Serialize};
use std::{collections::HashMap, fs, net::Ipv4Addr, path::Path, time::Duration};

pub const MAX_DROPPED_FILE_BYTES: u64 = 2 * 1024 * 1024 * 1024;

#[derive(Debug, Deserialize)]
pub struct DroppedFileUpload {
    pub server_url: String,
    pub endpoint: String,
    pub paths: Vec<String>,
    pub fields: HashMap<String, String>,
    pub token: String,
}

#[derive(Debug, Serialize)]
pub struct DroppedFileUploadResult {
    pub status: u16,
    pub content_type: String,
    pub body: String,
}

// upload_dropped_files 通过 Rust 的 multipart 文件流上传原生拖放文件，避免把大 ZIP 读入 WebView 内存。
// 参数说明：request 包含已验证服务地址、站内接口、拖放路径、表单字段和 Bearer 令牌。
// 返回说明：返回 HTTP 状态、内容类型和 JSON 错误/结果正文；单文件超过 2 GiB 时拒绝上传。
#[tauri::command]
pub async fn upload_dropped_files(
    request: DroppedFileUpload,
) -> Result<DroppedFileUploadResult, String> {
    let url = api_url(&request.server_url, &request.endpoint)?;
    if request.paths.len() > 100 {
        return Err(format!(
            "本次拖入了 {} 张图片，一次最多上传 100 张，请分批上传",
            request.paths.len()
        ));
    }
    if request.token.trim().is_empty() || request.paths.is_empty() {
        return Err("拖放上传参数无效".to_string());
    }

    let mut form = Form::new();
    for (name, value) in request.fields {
        form = form.text(name, value);
    }
    for path in request.paths {
        let file_path = Path::new(&path);
        let metadata =
            fs::metadata(file_path).map_err(|error| format!("无法读取拖入文件：{error}"))?;
        if !metadata.is_file() {
            return Err("拖入的路径不是文件".to_string());
        }
        if metadata.len() > MAX_DROPPED_FILE_BYTES {
            return Err("拖入文件不能超过 2 GiB".to_string());
        }
        form = form
            .file("file", file_path)
            .await
            .map_err(|error| format!("读取拖入文件失败：{error}"))?;
    }

    let response = reqwest::Client::builder()
        .connect_timeout(Duration::from_secs(15))
        .timeout(Duration::from_secs(2 * 60 * 60))
        .build()
        .map_err(|error| format!("创建上传连接失败：{error}"))?
        .post(url)
        .bearer_auth(request.token)
        .multipart(form)
        .send()
        .await
        .map_err(|error| format!("拖放上传失败：{error}"))?;
    let status = response.status().as_u16();
    let content_type = response
        .headers()
        .get(reqwest::header::CONTENT_TYPE)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default()
        .to_string();
    let body = response
        .text()
        .await
        .map_err(|error| format!("读取上传响应失败：{error}"))?;
    Ok(DroppedFileUploadResult {
        status,
        content_type,
        body,
    })
}

fn api_url(server_url: &str, endpoint: &str) -> Result<reqwest::Url, String> {
    if !endpoint.starts_with("/api/v1/") || endpoint.contains("..") {
        return Err("拖放上传接口无效".to_string());
    }
    let base = reqwest::Url::parse(server_url).map_err(|_| "服务地址格式不正确".to_string())?;
    if base.scheme() != "http" || base.username() != "" || base.password().is_some() {
        return Err("拖放上传仅支持内网 HTTP 服务".to_string());
    }
    let host = base
        .host_str()
        .ok_or_else(|| "服务地址缺少主机".to_string())?;
    let ip = host
        .parse::<Ipv4Addr>()
        .map_err(|_| "服务地址必须是 IPv4 地址".to_string())?;
    if !(ip.is_loopback()
        || ip.octets()[0] == 10
        || ip.octets()[0] == 192 && ip.octets()[1] == 168
        || ip.octets()[0] == 172 && (16..=31).contains(&ip.octets()[1]))
    {
        return Err("服务地址必须是本机或局域网 IPv4 地址".to_string());
    }
    base.join(endpoint)
        .map_err(|_| "拖放上传接口无效".to_string())
}
