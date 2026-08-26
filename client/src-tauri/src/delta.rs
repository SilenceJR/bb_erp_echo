//! Zstandard reference-prefix patches shared by the desktop updater and release CI.
//!
//! A patch is a normal zstd frame encoded with the previous executable as its
//! reference prefix.  It is deliberately not a general-purpose archive: callers
//! must verify the source and rebuilt SHA-256 values before replacing a binary.

use sha2::{Digest, Sha256};
use std::{
    fs,
    io::{self, Cursor, Read, Write},
    path::Path,
};

pub const ALGORITHM: &str = "zstd-patch-from-v1";

pub fn sha256_file(path: &Path) -> io::Result<String> {
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

pub fn create_patch(old: &Path, new: &Path, output: &Path) -> io::Result<()> {
    let base = fs::read(old)?;
    let target = fs::read(new)?;
    let file = fs::File::create(output)?;
    let mut encoder = zstd::stream::write::Encoder::with_ref_prefix(file, 19, &base)?;
    encoder.write_all(&target)?;
    encoder.finish()?;
    Ok(())
}

pub fn apply_patch(old: &Path, patch: &Path, output: &Path) -> io::Result<()> {
    let base = fs::read(old)?;
    let patch_data = fs::read(patch)?;
    let mut decoder = zstd::stream::read::Decoder::with_ref_prefix(Cursor::new(patch_data), &base)?;
    let mut rebuilt = fs::File::create(output)?;
    io::copy(&mut decoder, &mut rebuilt)?;
    rebuilt.flush()?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_path(name: &str) -> std::path::PathBuf {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir().join(format!("bb-erp-delta-{nonce}-{name}"))
    }

    #[test]
    fn rebuilds_target_with_matching_prefix() {
        let old = temp_path("old.exe");
        let new = temp_path("new.exe");
        let patch = temp_path("delta.zst");
        let rebuilt = temp_path("rebuilt.exe");
        fs::write(&old, b"BB-ERP binary data: old\0old\0old").unwrap();
        fs::write(&new, b"BB-ERP binary data: new\0old\0old").unwrap();
        create_patch(&old, &new, &patch).unwrap();
        apply_patch(&old, &patch, &rebuilt).unwrap();
        assert_eq!(sha256_file(&new).unwrap(), sha256_file(&rebuilt).unwrap());
        let _ = fs::remove_file(old);
        let _ = fs::remove_file(new);
        let _ = fs::remove_file(patch);
        let _ = fs::remove_file(rebuilt);
    }

    #[test]
    fn rejects_a_different_prefix() {
        let old = temp_path("old.exe");
        let wrong = temp_path("wrong.exe");
        let new = temp_path("new.exe");
        let patch = temp_path("delta.zst");
        let rebuilt = temp_path("rebuilt.exe");
        let mut base = Vec::with_capacity(512 * 1024);
        let mut value = 0x1357_9bdf_u32;
        for _ in 0..(512 * 1024) {
            value = value.wrapping_mul(1_664_525).wrapping_add(1_013_904_223);
            base.push((value >> 16) as u8);
        }
        let mut target = base.clone();
        target[17_321] ^= 0x7f;
        let wrong_data: Vec<u8> = base.iter().map(|byte| byte ^ 0xff).collect();
        fs::write(&old, &base).unwrap();
        fs::write(&wrong, wrong_data).unwrap();
        fs::write(&new, &target).unwrap();
        create_patch(&old, &new, &patch).unwrap();
        // zstd accepts a reference prefix with the same shape but reconstructs
        // different bytes. The updater rejects that result with target SHA-256.
        apply_patch(&wrong, &patch, &rebuilt).unwrap();
        assert_ne!(sha256_file(&new).unwrap(), sha256_file(&rebuilt).unwrap());
        for path in [old, wrong, new, patch, rebuilt] {
            let _ = fs::remove_file(path);
        }
    }

    #[test]
    fn truncated_patch_cannot_rebuild_the_expected_target() {
        let old = temp_path("truncated-old.exe");
        let new = temp_path("truncated-new.exe");
        let patch = temp_path("truncated.patch");
        let rebuilt = temp_path("truncated-rebuilt.exe");
        let base = vec![0x5a; 256 * 1024];
        let mut target = base.clone();
        target[1024] = 0x41;
        fs::write(&old, &base).unwrap();
        fs::write(&new, &target).unwrap();
        create_patch(&old, &new, &patch).unwrap();
        let mut bytes = fs::read(&patch).unwrap();
        bytes.truncate(bytes.len() - 3);
        fs::write(&patch, bytes).unwrap();
        match apply_patch(&old, &patch, &rebuilt) {
            Err(_) => {}
            Ok(()) => assert_ne!(sha256_file(&new).unwrap(), sha256_file(&rebuilt).unwrap()),
        }
        for path in [old, new, patch, rebuilt] {
            let _ = fs::remove_file(path);
        }
    }
}
