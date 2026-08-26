#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    if bb_erp_client_lib::update::try_run_update_helper() {
        return;
    }
    bb_erp_client_lib::run()
}
