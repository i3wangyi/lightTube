#define _GNU_SOURCE
#include "hwcaps.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <unistd.h>
#include <dlfcn.h>
#include <glob.h>
#include <errno.h>

// VA constants copied from <va/va.h> and <va/va_drm.h> to prevent build-time dependencies.
#define VA_STATUS_SUCCESS 0

// Entrypoint for VLD (video decode)
#define VAEntrypointVLD 1

// Profile constants from <va/va.h>
#define VAProfileH264Baseline 5
#define VAProfileH264Main 6
#define VAProfileH264High 7
#define VAProfileH264ConstrainedBaseline 13
#define VAProfileH264MultiviewHigh 15
#define VAProfileH264StereoHigh 16
#define VAProfileH264High10 36
#define VAProfileH264High422 40

#define VAProfileHEVCMain 17
#define VAProfileHEVCMain10 18
#define VAProfileHEVCMain12 23
#define VAProfileHEVCMain422_10 24
#define VAProfileHEVCMain422_12 25
#define VAProfileHEVCMain444 26
#define VAProfileHEVCMain444_10 27
#define VAProfileHEVCMain444_12 28
#define VAProfileHEVCSccMain 29
#define VAProfileHEVCSccMain10 30
#define VAProfileHEVCSccMain444 31
#define VAProfileHEVCSccMain444_10 34

#define VAProfileVP9Profile0 19
#define VAProfileVP9Profile1 20
#define VAProfileVP9Profile2 21
#define VAProfileVP9Profile3 22

#define VAProfileAV1Profile0 32
#define VAProfileAV1Profile1 33
#define VAProfileAV1Profile2 39

typedef void *VADisplay;
typedef int VAStatus;

int query_decode_caps(va_decode_caps_t *caps, char *err_buf, size_t err_len) {
    if (!caps) return -1;
    memset(caps, 0, sizeof(*caps));

    void *h_va = dlopen("libva.so.2", RTLD_NOW);
    if (!h_va) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "cannot load libva.so.2: %s", dlerror());
        }
        return -1;
    }

    void *h_drm = dlopen("libva-drm.so.2", RTLD_NOW);
    if (!h_drm) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "cannot load libva-drm.so.2: %s", dlerror());
        }
        dlclose(h_va);
        return -1;
    }

    VADisplay (*fn_vaGetDisplayDRM)(int) = (VADisplay (*)(int))dlsym(h_drm, "vaGetDisplayDRM");
    VAStatus (*fn_vaInitialize)(VADisplay, int *, int *) = (VAStatus (*)(VADisplay, int *, int *))dlsym(h_va, "vaInitialize");
    VAStatus (*fn_vaTerminate)(VADisplay) = (VAStatus (*)(VADisplay))dlsym(h_va, "vaTerminate");
    const char *(*fn_vaQueryVendorString)(VADisplay) = (const char *(*)(VADisplay))dlsym(h_va, "vaQueryVendorString");
    int (*fn_vaMaxNumProfiles)(VADisplay) = (int (*)(VADisplay))dlsym(h_va, "vaMaxNumProfiles");
    int (*fn_vaMaxNumEntrypoints)(VADisplay) = (int (*)(VADisplay))dlsym(h_va, "vaMaxNumEntrypoints");
    VAStatus (*fn_vaQueryConfigProfiles)(VADisplay, int *, int *) = (VAStatus (*)(VADisplay, int *, int *))dlsym(h_va, "vaQueryConfigProfiles");
    VAStatus (*fn_vaQueryConfigEntrypoints)(VADisplay, int, int *, int *) = (VAStatus (*)(VADisplay, int, int *, int *))dlsym(h_va, "vaQueryConfigEntrypoints");

    if (!fn_vaGetDisplayDRM || !fn_vaInitialize || !fn_vaTerminate || !fn_vaQueryVendorString ||
        !fn_vaMaxNumProfiles || !fn_vaMaxNumEntrypoints || !fn_vaQueryConfigProfiles || !fn_vaQueryConfigEntrypoints) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "missing required libva/drm symbols");
        }
        dlclose(h_drm);
        dlclose(h_va);
        return -1;
    }

    glob_t g;
    if (glob("/dev/dri/renderD*", 0, NULL, &g) != 0 || g.gl_pathc == 0) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "no /dev/dri/renderD* node found");
        }
        dlclose(h_drm);
        dlclose(h_va);
        return -1;
    }

    int fd = open(g.gl_pathv[0], O_RDWR | O_CLOEXEC);
    globfree(&g);
    if (fd < 0) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "failed to open render node: %s", strerror(errno));
        }
        dlclose(h_drm);
        dlclose(h_va);
        return -1;
    }

    VADisplay dpy = fn_vaGetDisplayDRM(fd);
    if (!dpy) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "vaGetDisplayDRM failed");
        }
        close(fd);
        dlclose(h_drm);
        dlclose(h_va);
        return -1;
    }

    int major = 0, minor = 0;
    VAStatus st = fn_vaInitialize(dpy, &major, &minor);
    if (st != VA_STATUS_SUCCESS) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "vaInitialize failed (code %d)", st);
        }
        close(fd);
        dlclose(h_drm);
        dlclose(h_va);
        return -1;
    }

    const char *vendor = fn_vaQueryVendorString(dpy);
    if (vendor) {
        if (strstr(vendor, "iHD")) {
            strncpy(caps->driver, "iHD", sizeof(caps->driver) - 1);
        } else if (strstr(vendor, "i965")) {
            strncpy(caps->driver, "i965", sizeof(caps->driver) - 1);
        } else if (strstr(vendor, "radeonsi")) {
            strncpy(caps->driver, "radeonsi", sizeof(caps->driver) - 1);
        } else if (strstr(vendor, "iris")) {
            strncpy(caps->driver, "iris", sizeof(caps->driver) - 1);
        } else if (strstr(vendor, "nouveau")) {
            strncpy(caps->driver, "nouveau", sizeof(caps->driver) - 1);
        } else {
            strncpy(caps->driver, vendor, sizeof(caps->driver) - 1);
        }
    }

    int max_p = fn_vaMaxNumProfiles(dpy);
    int max_e = fn_vaMaxNumEntrypoints(dpy);
    if (max_p <= 0) max_p = 64;
    if (max_e <= 0) max_e = 32;

    int *profiles = (int *)malloc(max_p * sizeof(int));
    int *entrypoints = (int *)malloc(max_e * sizeof(int));
    int num_profiles = 0;

    if (profiles && entrypoints) {
        st = fn_vaQueryConfigProfiles(dpy, profiles, &num_profiles);
        if (st == VA_STATUS_SUCCESS) {
            for (int i = 0; i < num_profiles; i++) {
                int profile = profiles[i];
                int num_entrypoints = 0;
                st = fn_vaQueryConfigEntrypoints(dpy, profile, entrypoints, &num_entrypoints);
                if (st != VA_STATUS_SUCCESS) continue;

                int has_vld = 0;
                for (int j = 0; j < num_entrypoints; j++) {
                    if (entrypoints[j] == VAEntrypointVLD) {
                        has_vld = 1;
                        break;
                    }
                }
                if (!has_vld) continue;

                if (profile == VAProfileH264Baseline || profile == VAProfileH264Main ||
                    profile == VAProfileH264High || profile == VAProfileH264ConstrainedBaseline ||
                    profile == VAProfileH264MultiviewHigh || profile == VAProfileH264StereoHigh ||
                    profile == VAProfileH264High10 || profile == VAProfileH264High422) {
                    caps->h264 = 1;
                } else if (profile == VAProfileHEVCMain || profile == VAProfileHEVCMain10 ||
                           profile == VAProfileHEVCMain12 || profile == VAProfileHEVCMain422_10 ||
                           profile == VAProfileHEVCMain422_12 || profile == VAProfileHEVCMain444 ||
                           profile == VAProfileHEVCMain444_10 || profile == VAProfileHEVCMain444_12 ||
                           profile == VAProfileHEVCSccMain || profile == VAProfileHEVCSccMain10 ||
                           profile == VAProfileHEVCSccMain444 || profile == VAProfileHEVCSccMain444_10) {
                    caps->hevc = 1;
                } else if (profile == VAProfileVP9Profile0) {
                    caps->vp9 = 1;
                } else if (profile == VAProfileAV1Profile0) {
                    caps->av1 = 1;
                }
            }
        }
    }

    if (profiles) free(profiles);
    if (entrypoints) free(entrypoints);

    fn_vaTerminate(dpy);
    close(fd);
    dlclose(h_drm);
    dlclose(h_va);
    return 0;
}

int check_libmpv_loadable(char *err_buf, size_t err_len) {
    void *h = dlopen("libmpv.so.2", RTLD_NOW);
    if (!h) {
        if (err_buf && err_len > 0) {
            snprintf(err_buf, err_len, "%s", dlerror());
        }
        return -1;
    }
    dlclose(h);
    return 0;
}
