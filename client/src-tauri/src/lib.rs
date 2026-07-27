// run 启动 Tauri 桌面壳。
//
// 参数说明：无。
// 返回说明：Tauri 运行失败时会 panic，并输出错误原因。
pub fn run() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("运行博邦 ERP 桌面端失败");
}
