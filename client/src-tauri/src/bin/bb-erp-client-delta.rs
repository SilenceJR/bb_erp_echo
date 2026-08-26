use bb_erp_client_lib::delta::{apply_patch, create_patch, sha256_file};
use bb_erp_client_lib::update::verify_signature_with_public_key;
use serde_json::json;
use std::{env, path::PathBuf, process};

fn value(args: &[String], name: &str) -> Result<PathBuf, String> {
    let index = args
        .iter()
        .position(|arg| arg == name)
        .ok_or_else(|| format!("missing {name}"))?;
    args.get(index + 1)
        .map(PathBuf::from)
        .ok_or_else(|| format!("missing value for {name}"))
}

fn sha_value(args: &[String]) -> Option<String> {
    args.iter()
        .position(|arg| arg == "--expected-sha256")
        .and_then(|i| args.get(i + 1))
        .cloned()
}

fn string_value(args: &[String], name: &str) -> Result<String, String> {
    let index = args
        .iter()
        .position(|arg| arg == name)
        .ok_or_else(|| format!("missing {name}"))?;
    args.get(index + 1)
        .cloned()
        .ok_or_else(|| format!("missing value for {name}"))
}

fn run(args: &[String]) -> Result<serde_json::Value, String> {
    let action = args
        .first()
        .map(String::as_str)
        .ok_or("usage: create|verify|verify-signature")?;
    match action {
        "create" => {
            let old = value(args, "--old")?;
            let output = value(args, "--output")?;
            let new = value(args, "--new")?;
            create_patch(&old, &new, &output).map_err(|error| error.to_string())?;
            Ok(json!({
                "source_sha256": sha256_file(&old).map_err(|error| error.to_string())?,
                "target_sha256": sha256_file(&new).map_err(|error| error.to_string())?,
                "patch_size": std::fs::metadata(output).map_err(|error| error.to_string())?.len(),
            }))
        }
        "verify" => {
            let old = value(args, "--old")?;
            let output = value(args, "--output")?;
            let patch = value(args, "--patch")?;
            apply_patch(&old, &patch, &output).map_err(|error| error.to_string())?;
            let target_sha256 = sha256_file(&output).map_err(|error| error.to_string())?;
            if let Some(expected) = sha_value(args) {
                if !expected.eq_ignore_ascii_case(&target_sha256) {
                    return Err("rebuilt SHA-256 does not match --expected-sha256".into());
                }
            }
            Ok(
                json!({"source_sha256": sha256_file(&old).map_err(|error| error.to_string())?, "target_sha256": target_sha256}),
            )
        }
        "verify-signature" => {
            let public_key = string_value(args, "--public-key")?;
            let file = value(args, "--file")?;
            let signature =
                value(args, "--signature-file").or_else(|_| value(args, "--signature"))?;
            let payload = std::fs::read(&file).map_err(|error| error.to_string())?;
            let signature =
                std::fs::read_to_string(&signature).map_err(|error| error.to_string())?;
            verify_signature_with_public_key(&payload, &public_key, &signature)?;
            Ok(
                json!({"verified": true, "sha256": sha256_file(&file).map_err(|error| error.to_string())?}),
            )
        }
        _ => Err("usage: create|verify|verify-signature".into()),
    }
}

fn main() {
    let args: Vec<String> = env::args().skip(1).collect();
    match run(&args) {
        Ok(result) => println!("{}", result),
        Err(error) => {
            eprintln!("bb-erp-client-delta: {error}");
            process::exit(2);
        }
    }
}
