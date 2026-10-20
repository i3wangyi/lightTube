#define _GNU_SOURCE
#include "x11.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>
#include <dlfcn.h>
#include <unistd.h>
#include <malloc.h>

typedef void *Display_p;
typedef unsigned long Window_id;

typedef Display_p (*fn_XOpenDisplay_t)(const char *);
typedef int (*fn_XCloseDisplay_t)(Display_p);
typedef Window_id (*fn_XCreateSimpleWindow_t)(Display_p, Window_id, int, int, unsigned int, unsigned int, unsigned int, unsigned long, unsigned long);
typedef int (*fn_XMapWindow_t)(Display_p, Window_id);
typedef int (*fn_XUnmapWindow_t)(Display_p, Window_id);
typedef int (*fn_XMoveResizeWindow_t)(Display_p, Window_id, int, int, unsigned int, unsigned int);
typedef int (*fn_XDestroyWindow_t)(Display_p, Window_id);
typedef int (*fn_XFlush_t)(Display_p);

typedef struct x11_syms {
    void *lib;
    fn_XOpenDisplay_t XOpenDisplay;
    fn_XCloseDisplay_t XCloseDisplay;
    fn_XCreateSimpleWindow_t XCreateSimpleWindow;
    fn_XMapWindow_t XMapWindow;
    fn_XUnmapWindow_t XUnmapWindow;
    fn_XMoveResizeWindow_t XMoveResizeWindow;
    fn_XDestroyWindow_t XDestroyWindow;
    fn_XFlush_t XFlush;
} x11_syms;

typedef struct mpv_handle mpv_handle;
typedef enum mpv_format {
    MPV_FORMAT_NONE = 0,
    MPV_FORMAT_STRING = 1,
    MPV_FORMAT_FLAG = 3,
    MPV_FORMAT_INT64 = 4,
    MPV_FORMAT_DOUBLE = 5,
} mpv_format;

typedef struct mpv_syms {
    void *lib;
    mpv_handle *(*mpv_create)(void);
    int (*mpv_initialize)(mpv_handle *);
    void (*mpv_terminate_destroy)(mpv_handle *);
    int (*mpv_set_option_string)(mpv_handle *, const char *, const char *);
    int (*mpv_get_property)(mpv_handle *, const char *, mpv_format, void *);
    int (*mpv_set_property)(mpv_handle *, const char *, mpv_format, void *);
    char *(*mpv_get_property_string)(mpv_handle *, const char *);
    void (*mpv_free)(void *);
    int (*mpv_command_string)(mpv_handle *, const char *);
    int (*mpv_command_async)(mpv_handle *, uint64_t, const char **);
} mpv_syms;

struct x11_embed_context {
    x11_syms x11;
    mpv_syms mpv;

    Display_p display;
    Window_id parent_window;
    Window_id child_window;
    mpv_handle *mpv_ctx;

    pthread_mutex_t lock;
    bool is_playing;
    bool is_hidden;
};

static int load_x11_syms(x11_syms *s) {
    memset(s, 0, sizeof(*s));
    s->lib = dlopen("libX11.so.6", RTLD_NOW | RTLD_LOCAL);
    if (!s->lib) {
        s->lib = dlopen("libX11.so", RTLD_NOW | RTLD_LOCAL);
    }
    if (!s->lib) {
        return -1;
    }
    s->XOpenDisplay = (fn_XOpenDisplay_t)dlsym(s->lib, "XOpenDisplay");
    s->XCloseDisplay = (fn_XCloseDisplay_t)dlsym(s->lib, "XCloseDisplay");
    s->XCreateSimpleWindow = (fn_XCreateSimpleWindow_t)dlsym(s->lib, "XCreateSimpleWindow");
    s->XMapWindow = (fn_XMapWindow_t)dlsym(s->lib, "XMapWindow");
    s->XUnmapWindow = (fn_XUnmapWindow_t)dlsym(s->lib, "XUnmapWindow");
    s->XMoveResizeWindow = (fn_XMoveResizeWindow_t)dlsym(s->lib, "XMoveResizeWindow");
    s->XDestroyWindow = (fn_XDestroyWindow_t)dlsym(s->lib, "XDestroyWindow");
    s->XFlush = (fn_XFlush_t)dlsym(s->lib, "XFlush");

    if (!s->XOpenDisplay || !s->XCloseDisplay || !s->XCreateSimpleWindow ||
        !s->XMapWindow || !s->XUnmapWindow || !s->XMoveResizeWindow ||
        !s->XDestroyWindow || !s->XFlush) {
        dlclose(s->lib);
        s->lib = NULL;
        return -2;
    }
    return 0;
}

static int load_mpv_syms(mpv_syms *s) {
    memset(s, 0, sizeof(*s));
    s->lib = dlopen("libmpv.so.2", RTLD_NOW | RTLD_LOCAL);
    if (!s->lib) {
        s->lib = dlopen("libmpv.so", RTLD_NOW | RTLD_LOCAL);
    }
    if (!s->lib) {
        return -1;
    }
    s->mpv_create = (mpv_handle *(*)(void))dlsym(s->lib, "mpv_create");
    s->mpv_initialize = (int (*)(mpv_handle *))dlsym(s->lib, "mpv_initialize");
    s->mpv_terminate_destroy = (void (*)(mpv_handle *))dlsym(s->lib, "mpv_terminate_destroy");
    s->mpv_set_option_string = (int (*)(mpv_handle *, const char *, const char *))dlsym(s->lib, "mpv_set_option_string");
    s->mpv_get_property = (int (*)(mpv_handle *, const char *, mpv_format, void *))dlsym(s->lib, "mpv_get_property");
    s->mpv_set_property = (int (*)(mpv_handle *, const char *, mpv_format, void *))dlsym(s->lib, "mpv_set_property");
    s->mpv_get_property_string = (char *(*)(mpv_handle *, const char *))dlsym(s->lib, "mpv_get_property_string");
    s->mpv_free = (void (*)(void *))dlsym(s->lib, "mpv_free");
    s->mpv_command_string = (int (*)(mpv_handle *, const char *))dlsym(s->lib, "mpv_command_string");
    s->mpv_command_async = (int (*)(mpv_handle *, uint64_t, const char **))dlsym(s->lib, "mpv_command_async");

    if (!s->mpv_create || !s->mpv_initialize || !s->mpv_terminate_destroy ||
        !s->mpv_set_option_string || !s->mpv_get_property || !s->mpv_set_property ||
        !s->mpv_get_property_string || !s->mpv_free || !s->mpv_command_string ||
        !s->mpv_command_async) {
        dlclose(s->lib);
        s->lib = NULL;
        return -2;
    }
    return 0;
}

x11_embed_context *x11_embed_context_create(unsigned long parent_window) {
    if (parent_window == 0) return NULL;

    x11_embed_context *ctx = (x11_embed_context *)calloc(1, sizeof(*ctx));
    if (!ctx) return NULL;

    if (load_x11_syms(&ctx->x11) != 0) {
        free(ctx);
        return NULL;
    }

    if (load_mpv_syms(&ctx->mpv) != 0) {
        if (ctx->x11.lib) dlclose(ctx->x11.lib);
        free(ctx);
        return NULL;
    }

    pthread_mutex_init(&ctx->lock, NULL);
    ctx->parent_window = parent_window;

    // Use a separate Display connection (XOpenDisplay(NULL) via dlopen)
    // for child window calls so Gio's connection is not used from another thread.
    ctx->display = ctx->x11.XOpenDisplay(NULL);
    if (!ctx->display) {
        if (ctx->mpv.lib) dlclose(ctx->mpv.lib);
        if (ctx->x11.lib) dlclose(ctx->x11.lib);
        pthread_mutex_destroy(&ctx->lock);
        free(ctx);
        return NULL;
    }

    // Create a child window of the Gio window
    ctx->child_window = ctx->x11.XCreateSimpleWindow(
        ctx->display,
        ctx->parent_window,
        0, 0, 1, 1, 0, 0, 0
    );
    if (!ctx->child_window) {
        ctx->x11.XCloseDisplay(ctx->display);
        if (ctx->mpv.lib) dlclose(ctx->mpv.lib);
        if (ctx->x11.lib) dlclose(ctx->x11.lib);
        pthread_mutex_destroy(&ctx->lock);
        free(ctx);
        return NULL;
    }

    ctx->x11.XMapWindow(ctx->display, ctx->child_window);
    ctx->x11.XFlush(ctx->display);

    return ctx;
}

int x11_embed_player_start(x11_embed_context *ctx, const char *url, const char *title,
                           const char *ytdl_format, const char *cookies_file,
                           const char *hwdec,
                           int width_px, int height_px, int scale, int debug) {
    if (!ctx || !url) return -1;

    pthread_mutex_lock(&ctx->lock);

    // Stop existing playback if any
    if (ctx->mpv_ctx) {
        ctx->mpv.mpv_terminate_destroy(ctx->mpv_ctx);
        ctx->mpv_ctx = NULL;
    }

    ctx->mpv_ctx = ctx->mpv.mpv_create();
    if (!ctx->mpv_ctx) {
        pthread_mutex_unlock(&ctx->lock);
        return -2;
    }

    char wid_str[32];
    snprintf(wid_str, sizeof(wid_str), "%lu", (unsigned long)ctx->child_window);
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "wid", wid_str);
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "vo", "gpu");
    // Without these mpv picks Vulkan on Broadwell (hasvk), which used about
    // 500 MB RSS against about 140 MB for the OpenGL path.
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "gpu-api", "opengl");
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "gpu-context", "x11egl");

    const char *hwdec_opt = (hwdec && hwdec[0]) ? hwdec : "auto-safe";
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "hwdec", hwdec_opt);
    // The default demuxer cache (150 MiB ahead, 50 MiB back) was most of the
    // RSS during playback; 64 MiB is still about 100 s of 1080p H.264.
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "demuxer-max-bytes", "64MiB");
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "demuxer-max-back-bytes", "16MiB");

    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "osc", "no");
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "input-default-bindings", "no");
    ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "save-position-on-quit", "yes");

    if (title && title[0]) {
        ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "title", title);
    }
    if (ytdl_format && ytdl_format[0]) {
        ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "ytdl-format", ytdl_format);
    }
    if (cookies_file && cookies_file[0]) {
        char cookie_opt[512];
        snprintf(cookie_opt, sizeof(cookie_opt), "cookies=%s", cookies_file);
        ctx->mpv.mpv_set_option_string(ctx->mpv_ctx, "ytdl-raw-options", cookie_opt);
    }

    if (ctx->mpv.mpv_initialize(ctx->mpv_ctx) < 0) {
        ctx->mpv.mpv_terminate_destroy(ctx->mpv_ctx);
        ctx->mpv_ctx = NULL;
        pthread_mutex_unlock(&ctx->lock);
        return -3;
    }

    // Map child window and flush
    ctx->x11.XMapWindow(ctx->display, ctx->child_window);
    if (width_px > 0 && height_px > 0) {
        ctx->x11.XMoveResizeWindow(ctx->display, ctx->child_window, 0, 0, width_px, height_px);
    }
    ctx->x11.XFlush(ctx->display);

    const char *cmd[] = {"loadfile", url, NULL};
    ctx->mpv.mpv_command_async(ctx->mpv_ctx, 0, cmd);

    ctx->is_playing = true;
    ctx->is_hidden = false;

    pthread_mutex_unlock(&ctx->lock);
    return 0;
}

void x11_embed_player_stop(x11_embed_context *ctx) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);

    if (ctx->mpv_ctx) {
        ctx->mpv.mpv_terminate_destroy(ctx->mpv_ctx);
        ctx->mpv_ctx = NULL;
    }
    if (ctx->display && ctx->child_window) {
        ctx->x11.XUnmapWindow(ctx->display, ctx->child_window);
        ctx->x11.XFlush(ctx->display);
    }
    ctx->is_playing = false;

    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_set_rect(x11_embed_context *ctx, int x_px, int y_px, int w_px, int h_px, int scale) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    if (ctx->display && ctx->child_window) {
        if (w_px <= 0 || h_px <= 0) {
            ctx->is_hidden = true;
            ctx->x11.XUnmapWindow(ctx->display, ctx->child_window);
        } else {
            ctx->is_hidden = false;
            ctx->x11.XMapWindow(ctx->display, ctx->child_window);
            ctx->x11.XMoveResizeWindow(ctx->display, ctx->child_window, x_px, y_px, (unsigned int)w_px, (unsigned int)h_px);
        }
        ctx->x11.XFlush(ctx->display);
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_set_hidden(x11_embed_context *ctx, bool hidden) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    ctx->is_hidden = hidden;
    if (ctx->display && ctx->child_window) {
        if (hidden) {
            ctx->x11.XUnmapWindow(ctx->display, ctx->child_window);
        } else {
            ctx->x11.XMapWindow(ctx->display, ctx->child_window);
        }
        ctx->x11.XFlush(ctx->display);
    }
    if (hidden && ctx->mpv_ctx) {
        int paused = 1;
        ctx->mpv.mpv_set_property(ctx->mpv_ctx, "pause", MPV_FORMAT_FLAG, &paused);
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_toggle_pause(x11_embed_context *ctx) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    if (ctx->mpv_ctx) {
        int paused = 0;
        if (ctx->mpv.mpv_get_property(ctx->mpv_ctx, "pause", MPV_FORMAT_FLAG, &paused) >= 0) {
            int next = !paused;
            ctx->mpv.mpv_set_property(ctx->mpv_ctx, "pause", MPV_FORMAT_FLAG, &next);
        }
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_set_pause(x11_embed_context *ctx, bool paused) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    if (ctx->mpv_ctx) {
        int p = paused ? 1 : 0;
        ctx->mpv.mpv_set_property(ctx->mpv_ctx, "pause", MPV_FORMAT_FLAG, &p);
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_seek(x11_embed_context *ctx, double seconds, bool relative) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    if (ctx->mpv_ctx) {
        char cmd[128];
        if (relative) {
            snprintf(cmd, sizeof(cmd), "seek %f relative", seconds);
        } else {
            snprintf(cmd, sizeof(cmd), "seek %f absolute", seconds);
        }
        ctx->mpv.mpv_command_string(ctx->mpv_ctx, cmd);
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_set_volume(x11_embed_context *ctx, double volume) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    if (ctx->mpv_ctx) {
        if (volume < 0.0) volume = 0.0;
        if (volume > 100.0) volume = 100.0;
        ctx->mpv.mpv_set_property(ctx->mpv_ctx, "volume", MPV_FORMAT_DOUBLE, &volume);
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_toggle_mute(x11_embed_context *ctx) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    if (ctx->mpv_ctx) {
        int muted = 0;
        if (ctx->mpv.mpv_get_property(ctx->mpv_ctx, "mute", MPV_FORMAT_FLAG, &muted) >= 0) {
            int next = !muted;
            ctx->mpv.mpv_set_property(ctx->mpv_ctx, "mute", MPV_FORMAT_FLAG, &next);
        }
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_player_get_status(x11_embed_context *ctx, embed_status *status) {
    memset(status, 0, sizeof(*status));
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    if (ctx->mpv_ctx) {
        status->is_playing = ctx->is_playing;
        ctx->mpv.mpv_get_property(ctx->mpv_ctx, "time-pos", MPV_FORMAT_DOUBLE, &status->time_pos);
        ctx->mpv.mpv_get_property(ctx->mpv_ctx, "duration", MPV_FORMAT_DOUBLE, &status->duration);

        int paused = 0;
        if (ctx->mpv.mpv_get_property(ctx->mpv_ctx, "pause", MPV_FORMAT_FLAG, &paused) >= 0) {
            status->paused = paused ? true : false;
        }

        int mute = 0;
        if (ctx->mpv.mpv_get_property(ctx->mpv_ctx, "mute", MPV_FORMAT_FLAG, &mute) >= 0) {
            status->muted = mute ? true : false;
        }

        ctx->mpv.mpv_get_property(ctx->mpv_ctx, "volume", MPV_FORMAT_DOUBLE, &status->volume);

        char *hwdec = ctx->mpv.mpv_get_property_string(ctx->mpv_ctx, "hwdec-current");
        if (hwdec) {
            strncpy(status->hwdec_current, hwdec, sizeof(status->hwdec_current) - 1);
            ctx->mpv.mpv_free(hwdec);
        }
        char *vcodec = ctx->mpv.mpv_get_property_string(ctx->mpv_ctx, "video-codec");
        if (!vcodec) {
            vcodec = ctx->mpv.mpv_get_property_string(ctx->mpv_ctx, "video-format");
        }
        if (vcodec) {
            strncpy(status->video_codec, vcodec, sizeof(status->video_codec) - 1);
            ctx->mpv.mpv_free(vcodec);
        }

        int eof = 0;
        if (ctx->mpv.mpv_get_property(ctx->mpv_ctx, "eof-reached", MPV_FORMAT_FLAG, &eof) >= 0) {
            status->eof_reached = eof ? true : false;
        }
    }
    pthread_mutex_unlock(&ctx->lock);
}

void x11_embed_context_destroy(x11_embed_context *ctx) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);

    if (ctx->mpv_ctx) {
        ctx->mpv.mpv_terminate_destroy(ctx->mpv_ctx);
        ctx->mpv_ctx = NULL;
    }
    if (ctx->display && ctx->child_window) {
        ctx->x11.XDestroyWindow(ctx->display, ctx->child_window);
        ctx->child_window = 0;
    }
    if (ctx->display) {
        ctx->x11.XFlush(ctx->display);
        ctx->x11.XCloseDisplay(ctx->display);
        ctx->display = NULL;
    }
    if (ctx->mpv.lib) {
        dlclose(ctx->mpv.lib);
        ctx->mpv.lib = NULL;
    }
    if (ctx->x11.lib) {
        dlclose(ctx->x11.lib);
        ctx->x11.lib = NULL;
    }
    pthread_mutex_unlock(&ctx->lock);
    pthread_mutex_destroy(&ctx->lock);
    free(ctx);
    malloc_trim(0);
}
