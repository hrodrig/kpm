# OpenBSD port for kpm (Kubernetes pods metrics CLI)
# Copy to /usr/ports/sysutils/kpm/ and run: make makesum
# Submit: send diff to ports@openbsd.org

COMMENT =	Kubernetes pods metrics CLI (sample to SQLite)
DISTNAME =	kpm_v0.9.0_openbsd_${MACHINE_ARCH:S/aarch64/arm64/}
PKGNAME =	kpm-0.9.0
CATEGORIES =	sysutils
HOMEPAGE =	https://github.com/hrodrig/kpm
MAINTAINER =	hrodrig@usb.ve

# MIT
PERMIT_PACKAGE =	Yes
PERMIT_DISTFILES =	Yes

# Local test before release: copy tarball to DISTDIR or use
#   make fetch MASTER_SITES=file:///absolute/path/to/dir/
# See README.md "Test with a local tarball".
MASTER_SITES =	https://github.com/hrodrig/kpm/releases/download/v0.9.0/
DISTFILES =	kpm_v0.9.0_openbsd_${MACHINE_ARCH:S/aarch64/arm64/}.tar.gz
EXTRACT_SUFX =	.tar.gz

ONLY_FOR_ARCHS =	amd64 arm64 aarch64
NO_BUILD =	Yes
NO_TEST =	Yes
PKG_ARCH =	*

do-install:
	${INSTALL_PROGRAM} ${WRKDIR}/kpm ${PREFIX}/bin/kpm
	${INSTALL_PROGRAM} ${WRKDIR}/kubectl-kpm ${PREFIX}/bin/kubectl-kpm
	${INSTALL_MAN} ${WRKDIR}/share/man/man1/kpm.1 ${PREFIX}/man/man1/kpm.1
	${INSTALL_MAN} ${WRKDIR}/share/man/man1/kubectl-kpm.1 ${PREFIX}/man/man1/kubectl-kpm.1
	${INSTALL_DATA_DIR} ${PREFIX}/share/doc/kpm
	${INSTALL_DATA} ${WRKDIR}/share/doc/kpm/LICENSE ${PREFIX}/share/doc/kpm/
	${INSTALL_DATA_DIR} ${PREFIX}/share/examples/kpm
	${INSTALL_DATA} ${WRKDIR}/share/examples/kpm/kpm.sample.yml ${PREFIX}/share/examples/kpm/

.include <bsd.port.mk>
