# OpenBSD port — kpm

## Test with a local tarball

1. From the **kpm** repo root:

   ```bash
   make port-openbsd-sync
   make dist-openbsd
   ```

2. Point `MASTER_SITES` at the directory containing
   `kpm_v<version>_openbsd_<arch>.tar.gz`, then `make makesum && make`.
