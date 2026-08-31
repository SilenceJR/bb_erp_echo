pub mod discovery;
pub mod save;
pub mod update;

// run 启动 Tauri 桌面壳。
//
// 参数说明：无。
// 返回说明：Tauri 运行失败时会 panic，并输出错误原因。
pub fn run() {
    let mut builder = tauri::Builder::default()
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_window_state::Builder::default().build())
        .manage(update::UpdateEngine::default())
        .invoke_handler(tauri::generate_handler![
            discovery::discover_servers,
            discovery::test_server_connection,
            save::save_api_file,
            update::client_update_check,
            update::client_update_apply,
            update::client_update_status,
        ]);
    if let Some(public_key) = update::update_public_key() {
        builder = builder.plugin(
            tauri_plugin_updater::Builder::new()
                .pubkey(public_key)
                .build(),
        );
    }
    builder
        .build(tauri::generate_context!())
        .expect("初始化博邦 ERP 桌面端失败")
        .run(|_handle, event| {
            if matches!(event, tauri::RunEvent::Ready) {
                update::mark_ready_from_args();
            }
        });
}
