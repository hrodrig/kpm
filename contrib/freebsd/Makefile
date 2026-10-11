PORTNAME=	kpm
PORTVERSION=	0.2.0
DISTVERSIONPREFIX=	v
CATEGORIES=	sysutils
# Local test before release: copy the tarball into DISTDIR (see `make -V DISTDIR`
# from this port directory) using the same name as DISTFILES, or run e.g.
#   make fetch MASTER_SITES=file:///absolute/path/to/dir/
# See README.md "Test with a local distfile".
MASTER_SITES=	https://github.com/hrodrig/kpm/releases/download/v${PORTVERSION}/
DISTFILES=	kpm_v${PORTVERSION}_freebsd_${ARCH:S/aarch64/arm64/}.tar.gz
EXTRACT_SUFX=	.tar.gz

MAINTAINER=	hrodrig@usb.ve
COMMENT=	Kubernetes pods metrics CLI (sample to SQLite, rightsizing)
WWW=		https://github.com/hrodrig/kpm
LICENSE=	MIT

NO_BUILD=	yes
NO_ARCH=	yes

do-install:
	${INSTALL_PROGRAM} ${WRKDIR}/kpm ${STAGEDIR}${PREFIX}/bin/kpm
	${INSTALL_PROGRAM} ${WRKDIR}/kubectl-kpm ${STAGEDIR}${PREFIX}/bin/kubectl-kpm
	${MKDIR} ${STAGEDIR}${PREFIX}/share/man/man1
	${INSTALL_MAN} ${WRKDIR}/share/man/man1/kpm.1 ${STAGEDIR}${PREFIX}/share/man/man1/
	${INSTALL_MAN} ${WRKDIR}/share/man/man1/kubectl-kpm.1 ${STAGEDIR}${PREFIX}/share/man/man1/
	${MKDIR} ${STAGEDIR}${PREFIX}/share/doc/kpm
	${INSTALL_DATA} ${WRKDIR}/share/doc/kpm/LICENSE ${STAGEDIR}${PREFIX}/share/doc/kpm/
	${MKDIR} ${STAGEDIR}${PREFIX}/share/examples/kpm
	${INSTALL_DATA} ${WRKDIR}/share/examples/kpm/kpm.sample.yml ${STAGEDIR}${PREFIX}/share/examples/kpm/

.include <bsd.port.mk>
