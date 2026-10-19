#define _GNU_SOURCE
#include "embed.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>
#include <dlfcn.h>
#include <unistd.h>
#include <malloc.h>

#include <wayland-client.h>
#include <wayland-egl.h>
#include <EGL/egl.h>
#include <GL/gl.h>

#include <mpv/client.h>
#include <mpv/render.h>
#include <mpv/render_gl.h>

// Dynamic function pointers for libmpv loaded via dlopen
static void *mpv_lib_handle = NULL;

static mpv_handle *(*fn_mpv_create)(void) = NULL;
static int (*fn_mpv_initialize)(mpv_handle *ctx) = NULL;
static void (*fn_mpv_terminate_destroy)(mpv_handle *ctx) = NULL;
static int (*fn_mpv_set_option_string)(mpv_handle *ctx, const char *name, const char *data) = NULL;
static int (*fn_mpv_get_property)(mpv_handle *ctx, const char *name, mpv_format format, void *data) = NULL;
static int (*fn_mpv_set_property)(mpv_handle *ctx, const char *name, mpv_format format, void *data) = NULL;
static char *(*fn_mpv_get_property_string)(mpv_handle *ctx, const char *name) = NULL;
static void (*fn_mpv_free)(void *data) = NULL;
static int (*fn_mpv_command_string)(mpv_handle *ctx, const char *args) = NULL;
static int (*fn_mpv_command_async)(mpv_handle *ctx, uint64_t reply_userdata, const char **args) = NULL;

static int (*fn_mpv_render_context_create)(mpv_render_context **res, mpv_handle *mpv, mpv_render_param *params) = NULL;
static void (*fn_mpv_render_context_set_update_callback)(mpv_render_context *ctx, mpv_render_update_fn cb, void *cb_ctx) = NULL;
static uint64_t (*fn_mpv_render_context_update)(mpv_render_context *ctx) = NULL;
static int (*fn_mpv_render_context_render)(mpv_render_context *ctx, mpv_render_param *params) = NULL;
static void (*fn_mpv_render_context_free)(mpv_render_context *ctx) = NULL;

static int load_mpv_symbols(void) {
    if (mpv_lib_handle) return 0;
    mpv_lib_handle = dlopen("libmpv.so.2", RTLD_NOW);
    if (!mpv_lib_handle) {
        return -1;
    }
    fn_mpv_create = (mpv_handle *(*)(void))dlsym(mpv_lib_handle, "mpv_create");
    fn_mpv_initialize = (int (*)(mpv_handle *))dlsym(mpv_lib_handle, "mpv_initialize");
    fn_mpv_terminate_destroy = (void (*)(mpv_handle *))dlsym(mpv_lib_handle, "mpv_terminate_destroy");
    fn_mpv_set_option_string = (int (*)(mpv_handle *, const char *, const char *))dlsym(mpv_lib_handle, "mpv_set_option_string");
    fn_mpv_get_property = (int (*)(mpv_handle *, const char *, mpv_format, void *))dlsym(mpv_lib_handle, "mpv_get_property");
    fn_mpv_set_property = (int (*)(mpv_handle *, const char *, mpv_format, void *))dlsym(mpv_lib_handle, "mpv_set_property");
    fn_mpv_get_property_string = (char *(*)(mpv_handle *, const char *))dlsym(mpv_lib_handle, "mpv_get_property_string");
    fn_mpv_free = (void (*)(void *))dlsym(mpv_lib_handle, "mpv_free");
    fn_mpv_command_string = (int (*)(mpv_handle *, const char *))dlsym(mpv_lib_handle, "mpv_command_string");
    fn_mpv_command_async = (int (*)(mpv_handle *, uint64_t, const char **))dlsym(mpv_lib_handle, "mpv_command_async");

    fn_mpv_render_context_create = (int (*)(mpv_render_context **, mpv_handle *, mpv_render_param *))dlsym(mpv_lib_handle, "mpv_render_context_create");
    fn_mpv_render_context_set_update_callback = (void (*)(mpv_render_context *, mpv_render_update_fn, void *))dlsym(mpv_lib_handle, "mpv_render_context_set_update_callback");
    fn_mpv_render_context_update = (uint64_t (*)(mpv_render_context *))dlsym(mpv_lib_handle, "mpv_render_context_update");
    fn_mpv_render_context_render = (int (*)(mpv_render_context *, mpv_render_param *))dlsym(mpv_lib_handle, "mpv_render_context_render");
    fn_mpv_render_context_free = (void (*)(mpv_render_context *))dlsym(mpv_lib_handle, "mpv_render_context_free");

    if (!fn_mpv_create || !fn_mpv_initialize || !fn_mpv_terminate_destroy ||
        !fn_mpv_set_option_string || !fn_mpv_get_property || !fn_mpv_set_property ||
        !fn_mpv_get_property_string || !fn_mpv_free || !fn_mpv_command_string ||
        !fn_mpv_command_async || !fn_mpv_render_context_create ||
        !fn_mpv_render_context_set_update_callback || !fn_mpv_render_context_update ||
        !fn_mpv_render_context_render || !fn_mpv_render_context_free) {
        dlclose(mpv_lib_handle);
        mpv_lib_handle = NULL;
        return -2;
    }
    return 0;
}

struct embed_context {
    // Wayland core
    struct wl_display *display;
    struct wl_surface *parent_surface;
    struct wl_event_queue *queue;
    struct wl_registry *registry;
    struct wl_compositor *compositor;
    struct wl_subcompositor *subcompositor;

    // Subsurface and EGL (created on play, destroyed on close)
    struct wl_surface *child_surface;
    struct wl_subsurface *subsurface;
    struct wl_egl_window *egl_window;

    EGLDisplay egl_display;
    EGLConfig egl_config;
    EGLContext egl_context;
    EGLSurface egl_surface;

    // mpv core & render context
    mpv_handle *mpv;
    mpv_render_context *render_ctx;

    // Render thread synchronization
    pthread_t render_thread;
    pthread_mutex_t lock;
    pthread_cond_t cond;
    pthread_cond_t init_cond;

    bool init_done;
    bool init_failed;
    bool render_thread_running;
    bool render_needed;

    // Geometry & state
    int width_px;
    int height_px;
    int scale;
    int pos_x_surf;
    int pos_y_surf;
    bool size_changed;
    bool pos_changed;
    bool is_hidden;
    int debug;
};

static void registry_global(void *data, struct wl_registry *reg, uint32_t name, const char *iface, uint32_t ver) {
    embed_context *ctx = (embed_context *)data;
    if (strcmp(iface, wl_compositor_interface.name) == 0) {
        uint32_t v = ver < 4 ? ver : 4;
        ctx->compositor = (struct wl_compositor *)wl_registry_bind(reg, name, &wl_compositor_interface, v);
    } else if (strcmp(iface, wl_subcompositor_interface.name) == 0) {
        ctx->subcompositor = (struct wl_subcompositor *)wl_registry_bind(reg, name, &wl_subcompositor_interface, 1);
    }
}

static void registry_global_remove(void *data, struct wl_registry *reg, uint32_t name) {
    (void)data;
    (void)reg;
    (void)name;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

static void *get_proc_address_egl(void *ctx, const char *name) {
    (void)ctx;
    void *proc = (void *)eglGetProcAddress(name);
    if (!proc) {
        proc = dlsym(RTLD_DEFAULT, name);
    }
    return proc;
}

static void on_mpv_update_callback(void *arg) {
    embed_context *ctx = (embed_context *)arg;
    pthread_mutex_lock(&ctx->lock);
    ctx->render_needed = true;
    pthread_cond_signal(&ctx->cond);
    pthread_mutex_unlock(&ctx->lock);
}

static void *render_thread_main(void *arg) {
    embed_context *ctx = (embed_context *)arg;

    // Dedicated OS thread locked to the EGL context.
    if (!eglMakeCurrent(ctx->egl_display, ctx->egl_surface, ctx->egl_surface, ctx->egl_context)) {
        pthread_mutex_lock(&ctx->lock);
        ctx->init_failed = true;
        pthread_cond_signal(&ctx->init_cond);
        pthread_mutex_unlock(&ctx->lock);
        return NULL;
    }

    eglSwapInterval(ctx->egl_display, 1);

    mpv_opengl_init_params gl_init = {
        .get_proc_address = get_proc_address_egl,
        .get_proc_address_ctx = NULL,
    };
    mpv_render_param params[] = {
        {MPV_RENDER_PARAM_API_TYPE, MPV_RENDER_API_TYPE_OPENGL},
        {MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &gl_init},
        {MPV_RENDER_PARAM_WL_DISPLAY, ctx->display},
        {0}
    };

    int err = fn_mpv_render_context_create(&ctx->render_ctx, ctx->mpv, params);
    if (err < 0) {
        pthread_mutex_lock(&ctx->lock);
        ctx->init_failed = true;
        pthread_cond_signal(&ctx->init_cond);
        pthread_mutex_unlock(&ctx->lock);
        return NULL;
    }

    fn_mpv_render_context_set_update_callback(ctx->render_ctx, on_mpv_update_callback, ctx);

    pthread_mutex_lock(&ctx->lock);
    ctx->init_done = true;
    pthread_cond_signal(&ctx->init_cond);
    pthread_mutex_unlock(&ctx->lock);

    bool last_hidden = false;

    while (1) {
        pthread_mutex_lock(&ctx->lock);
        while (ctx->render_thread_running && !ctx->render_needed && !ctx->size_changed && !ctx->pos_changed && ctx->is_hidden == last_hidden) {
            pthread_cond_wait(&ctx->cond, &ctx->lock);
        }
        if (!ctx->render_thread_running) {
            pthread_mutex_unlock(&ctx->lock);
            break;
        }

        ctx->render_needed = false;
        bool size_changed = ctx->size_changed;
        ctx->size_changed = false;
        bool pos_changed = ctx->pos_changed;
        ctx->pos_changed = false;
        int cur_w = ctx->width_px;
        int cur_h = ctx->height_px;
        int cur_x = ctx->pos_x_surf;
        int cur_y = ctx->pos_y_surf;
        bool hidden = ctx->is_hidden;
        pthread_mutex_unlock(&ctx->lock);

        if (hidden != last_hidden) {
            last_hidden = hidden;
            if (hidden) {
                if (ctx->child_surface) {
                    wl_surface_attach(ctx->child_surface, NULL, 0, 0);
                    wl_surface_commit(ctx->child_surface);
                    wl_display_flush(ctx->display);
                }
            }
        }

        if (pos_changed && ctx->subsurface) {
            wl_subsurface_set_position(ctx->subsurface, cur_x, cur_y);
            wl_display_flush(ctx->display);
        }
        if (size_changed && ctx->egl_window && cur_w > 0 && cur_h > 0) {
            wl_egl_window_resize(ctx->egl_window, cur_w, cur_h, 0, 0);
        }

        if (!hidden && cur_w > 0 && cur_h > 0 && ctx->render_ctx) {
            uint64_t flags = fn_mpv_render_context_update(ctx->render_ctx);
            if (flags & MPV_RENDER_UPDATE_FRAME) {
                mpv_opengl_fbo fbo = {
                    .fbo = 0,
                    .w = cur_w,
                    .h = cur_h,
                    .internal_format = 0,
                };
                int flip_y = 1;
                mpv_render_param rparams[] = {
                    {MPV_RENDER_PARAM_OPENGL_FBO, &fbo},
                    {MPV_RENDER_PARAM_FLIP_Y, &flip_y},
                    {0}
                };
                fn_mpv_render_context_render(ctx->render_ctx, rparams);
                eglSwapBuffers(ctx->egl_display, ctx->egl_surface);
                wl_display_dispatch_queue_pending(ctx->display, ctx->queue);
            }
        }
    }

    if (ctx->render_ctx) {
        fn_mpv_render_context_set_update_callback(ctx->render_ctx, NULL, NULL);
        fn_mpv_render_context_free(ctx->render_ctx);
        ctx->render_ctx = NULL;
    }

    return NULL;
}

embed_context *embed_context_create(void *wl_display, void *wl_surface) {
    if (!wl_display || !wl_surface) {
        return NULL;
    }

    embed_context *ctx = (embed_context *)calloc(1, sizeof(*ctx));
    if (!ctx) return NULL;

    ctx->display = (struct wl_display *)wl_display;
    ctx->parent_surface = (struct wl_surface *)wl_surface;
    pthread_mutex_init(&ctx->lock, NULL);
    pthread_cond_init(&ctx->cond, NULL);
    pthread_cond_init(&ctx->init_cond, NULL);

    // Private wl event queue
    ctx->queue = wl_display_create_queue(ctx->display);
    if (!ctx->queue) {
        free(ctx);
        return NULL;
    }

    ctx->registry = wl_display_get_registry(ctx->display);
    if (!ctx->registry) {
        wl_event_queue_destroy(ctx->queue);
        free(ctx);
        return NULL;
    }
    wl_proxy_set_queue((struct wl_proxy *)ctx->registry, ctx->queue);
    wl_registry_add_listener(ctx->registry, &registry_listener, ctx);

    // Dispatch the private queue to bind compositor and subcompositor
    wl_display_roundtrip_queue(ctx->display, ctx->queue);

    if (!ctx->compositor || !ctx->subcompositor) {
        wl_registry_destroy(ctx->registry);
        wl_event_queue_destroy(ctx->queue);
        free(ctx);
        return NULL;
    }

    wl_proxy_set_queue((struct wl_proxy *)ctx->compositor, ctx->queue);
    wl_proxy_set_queue((struct wl_proxy *)ctx->subcompositor, ctx->queue);

    return ctx;
}

void embed_context_destroy(embed_context *ctx) {
    if (!ctx) return;
    embed_player_stop(ctx);

    if (ctx->subcompositor) {
        wl_subcompositor_destroy(ctx->subcompositor);
        ctx->subcompositor = NULL;
    }
    if (ctx->compositor) {
        wl_compositor_destroy(ctx->compositor);
        ctx->compositor = NULL;
    }
    if (ctx->registry) {
        wl_registry_destroy(ctx->registry);
        ctx->registry = NULL;
    }
    if (ctx->queue) {
        wl_event_queue_destroy(ctx->queue);
        ctx->queue = NULL;
    }
    pthread_mutex_destroy(&ctx->lock);
    pthread_cond_destroy(&ctx->cond);
    pthread_cond_destroy(&ctx->init_cond);
    free(ctx);
    malloc_trim(0);
}

int embed_player_start(embed_context *ctx, const char *url, const char *title,
                       const char *ytdl_format, const char *cookies_file,
                       const char *hwdec,
                       int width_px, int height_px, int scale, int debug) {

    if (!ctx || !url) return -1;

    // 1. Stop any existing playback session
    embed_player_stop(ctx);

    // 2. Load libmpv dynamically on first play
    if (load_mpv_symbols() != 0) {
        return -11;
    }

    ctx->debug = debug;
    ctx->scale = scale > 0 ? scale : 1;
    ctx->width_px = width_px > 0 ? width_px : 1280;
    ctx->height_px = height_px > 0 ? height_px : 720;
    ctx->size_changed = false;
    ctx->pos_changed = false;
    ctx->is_hidden = false;

    // 3. Create child wl_surface on private queue
    ctx->child_surface = wl_compositor_create_surface(ctx->compositor);
    if (!ctx->child_surface) return -2;
    wl_proxy_set_queue((struct wl_proxy *)ctx->child_surface, ctx->queue);

    // 2. Create wl_subsurface on Gio parent surface
    ctx->subsurface = wl_subcompositor_get_subsurface(ctx->subcompositor, ctx->child_surface, ctx->parent_surface);
    if (!ctx->subsurface) {
        wl_surface_destroy(ctx->child_surface);
        ctx->child_surface = NULL;
        return -3;
    }
    wl_proxy_set_queue((struct wl_proxy *)ctx->subsurface, ctx->queue);
    wl_subsurface_set_desync(ctx->subsurface);

    // 3. Set empty input region so all pointer and key input goes to Gio
    struct wl_region *empty_region = wl_compositor_create_region(ctx->compositor);
    if (empty_region) {
        wl_proxy_set_queue((struct wl_proxy *)empty_region, ctx->queue);
        wl_surface_set_input_region(ctx->child_surface, empty_region);
        wl_region_destroy(empty_region);
    }

    // 4. Respect buffer scale
    wl_surface_set_buffer_scale(ctx->child_surface, ctx->scale);

    // 5. Create wl_egl_window
    ctx->egl_window = wl_egl_window_create(ctx->child_surface, ctx->width_px, ctx->height_px);
    if (!ctx->egl_window) {
        wl_subsurface_destroy(ctx->subsurface);
        wl_surface_destroy(ctx->child_surface);
        ctx->subsurface = NULL;
        ctx->child_surface = NULL;
        return -4;
    }

    // 6. EGL Display, Config, Context, Surface
    ctx->egl_display = eglGetDisplay((EGLNativeDisplayType)ctx->display);
    if (ctx->egl_display == EGL_NO_DISPLAY || !eglInitialize(ctx->egl_display, NULL, NULL)) {
        wl_egl_window_destroy(ctx->egl_window);
        wl_subsurface_destroy(ctx->subsurface);
        wl_surface_destroy(ctx->child_surface);
        ctx->egl_window = NULL;
        ctx->subsurface = NULL;
        ctx->child_surface = NULL;
        return -5;
    }

    eglBindAPI(EGL_OPENGL_API);

    static const EGLint config_attribs[] = {
        EGL_SURFACE_TYPE, EGL_WINDOW_BIT,
        EGL_RED_SIZE, 8,
        EGL_GREEN_SIZE, 8,
        EGL_BLUE_SIZE, 8,
        EGL_ALPHA_SIZE, 8,
        EGL_RENDERABLE_TYPE, EGL_OPENGL_BIT,
        EGL_NONE
    };
    EGLint num_configs = 0;
    if (!eglChooseConfig(ctx->egl_display, config_attribs, &ctx->egl_config, 1, &num_configs) || num_configs < 1) {
        wl_egl_window_destroy(ctx->egl_window);
        wl_subsurface_destroy(ctx->subsurface);
        wl_surface_destroy(ctx->child_surface);
        ctx->egl_window = NULL;
        ctx->subsurface = NULL;
        ctx->child_surface = NULL;
        return -6;
    }

    ctx->egl_context = eglCreateContext(ctx->egl_display, ctx->egl_config, EGL_NO_CONTEXT, NULL);
    ctx->egl_surface = eglCreateWindowSurface(ctx->egl_display, ctx->egl_config, (EGLNativeWindowType)ctx->egl_window, NULL);
    if (ctx->egl_context == EGL_NO_CONTEXT || ctx->egl_surface == EGL_NO_SURFACE) {
        if (ctx->egl_surface != EGL_NO_SURFACE) eglDestroySurface(ctx->egl_display, ctx->egl_surface);
        if (ctx->egl_context != EGL_NO_CONTEXT) eglDestroyContext(ctx->egl_display, ctx->egl_context);
        wl_egl_window_destroy(ctx->egl_window);
        wl_subsurface_destroy(ctx->subsurface);
        wl_surface_destroy(ctx->child_surface);
        ctx->egl_surface = EGL_NO_SURFACE;
        ctx->egl_context = EGL_NO_CONTEXT;
        ctx->egl_window = NULL;
        ctx->subsurface = NULL;
        ctx->child_surface = NULL;
        return -7;
    }

    // 7. mpv handle & options
    ctx->mpv = fn_mpv_create();
    if (!ctx->mpv) {
        embed_player_stop(ctx);
        return -8;
    }

    const char *hwdec_opt = (hwdec && hwdec[0]) ? hwdec : "auto-safe";
    fn_mpv_set_option_string(ctx->mpv, "hwdec", hwdec_opt);
    // The default demuxer cache (150 MiB ahead, 50 MiB back) was most of the
    // RSS during playback; 64 MiB is still about 100 s of 1080p H.264.
    fn_mpv_set_option_string(ctx->mpv, "demuxer-max-bytes", "64MiB");
    fn_mpv_set_option_string(ctx->mpv, "demuxer-max-back-bytes", "16MiB");

    fn_mpv_set_option_string(ctx->mpv, "vo", "libmpv");
    fn_mpv_set_option_string(ctx->mpv, "osc", "no");
    fn_mpv_set_option_string(ctx->mpv, "input-default-bindings", "no");
    fn_mpv_set_option_string(ctx->mpv, "save-position-on-quit", "yes");
    if (title && title[0]) {
        fn_mpv_set_option_string(ctx->mpv, "title", title);
    }
    if (ytdl_format && ytdl_format[0]) {
        fn_mpv_set_option_string(ctx->mpv, "ytdl-format", ytdl_format);
    }
    if (cookies_file && cookies_file[0]) {
        char cookie_opt[512];
        snprintf(cookie_opt, sizeof(cookie_opt), "cookies=%s", cookies_file);
        fn_mpv_set_option_string(ctx->mpv, "ytdl-raw-options", cookie_opt);
    }

    if (fn_mpv_initialize(ctx->mpv) < 0) {
        embed_player_stop(ctx);
        return -9;
    }

    // 8. Launch dedicated render thread
    ctx->init_done = false;
    ctx->init_failed = false;
    ctx->render_thread_running = true;

    pthread_create(&ctx->render_thread, NULL, render_thread_main, ctx);

    // Wait for render thread to initialize context
    pthread_mutex_lock(&ctx->lock);
    while (!ctx->init_done && !ctx->init_failed) {
        pthread_cond_wait(&ctx->init_cond, &ctx->lock);
    }
    bool failed = ctx->init_failed;
    pthread_mutex_unlock(&ctx->lock);

    if (failed) {
        embed_player_stop(ctx);
        return -10;
    }

    // 9. Start playback
    const char *cmd[] = {"loadfile", url, NULL};
    fn_mpv_command_async(ctx->mpv, 0, cmd);

    return 0;
}

void embed_player_stop(embed_context *ctx) {
    if (!ctx) return;

    if (ctx->render_thread_running) {
        pthread_mutex_lock(&ctx->lock);
        ctx->render_thread_running = false;
        pthread_cond_signal(&ctx->cond);
        pthread_mutex_unlock(&ctx->lock);

        pthread_join(ctx->render_thread, NULL);
    }

    if (ctx->mpv && fn_mpv_terminate_destroy) {
        fn_mpv_terminate_destroy(ctx->mpv);
        ctx->mpv = NULL;
    }

    if (mpv_lib_handle) {
        dlclose(mpv_lib_handle);
        mpv_lib_handle = NULL;
        fn_mpv_create = NULL;
        fn_mpv_initialize = NULL;
        fn_mpv_terminate_destroy = NULL;
        fn_mpv_set_option_string = NULL;
        fn_mpv_get_property = NULL;
        fn_mpv_set_property = NULL;
        fn_mpv_get_property_string = NULL;
        fn_mpv_free = NULL;
        fn_mpv_command_string = NULL;
        fn_mpv_command_async = NULL;
        fn_mpv_render_context_create = NULL;
        fn_mpv_render_context_set_update_callback = NULL;
        fn_mpv_render_context_update = NULL;
        fn_mpv_render_context_render = NULL;
        fn_mpv_render_context_free = NULL;
    }

    if (ctx->egl_surface != EGL_NO_SURFACE) {
        eglDestroySurface(ctx->egl_display, ctx->egl_surface);
        ctx->egl_surface = EGL_NO_SURFACE;
    }
    if (ctx->egl_context != EGL_NO_CONTEXT) {
        eglDestroyContext(ctx->egl_display, ctx->egl_context);
        ctx->egl_context = EGL_NO_CONTEXT;
    }
    ctx->egl_display = EGL_NO_DISPLAY;
    if (ctx->egl_window) {
        wl_egl_window_destroy(ctx->egl_window);
        ctx->egl_window = NULL;
    }
    if (ctx->subsurface) {
        wl_subsurface_destroy(ctx->subsurface);
        ctx->subsurface = NULL;
    }
    if (ctx->child_surface) {
        wl_surface_destroy(ctx->child_surface);
        ctx->child_surface = NULL;
    }

    if (ctx->display) {
        wl_display_flush(ctx->display);
    }
    malloc_trim(0);
}

void embed_player_set_rect(embed_context *ctx, int x_px, int y_px, int w_px, int h_px, int scale) {
    if (!ctx) return;
    int cur_scale = scale > 0 ? scale : 1;
    int x_surf = x_px / cur_scale;
    int y_surf = y_px / cur_scale;

    pthread_mutex_lock(&ctx->lock);
    if (ctx->pos_x_surf != x_surf || ctx->pos_y_surf != y_surf) {
        ctx->pos_x_surf = x_surf;
        ctx->pos_y_surf = y_surf;
        ctx->pos_changed = true;
    }
    if (ctx->width_px != w_px || ctx->height_px != h_px || ctx->scale != cur_scale) {
        ctx->width_px = w_px;
        ctx->height_px = h_px;
        ctx->scale = cur_scale;
        ctx->size_changed = true;
        ctx->render_needed = true;
    }
    pthread_cond_signal(&ctx->cond);
    pthread_mutex_unlock(&ctx->lock);
}

void embed_player_set_hidden(embed_context *ctx, bool hidden) {
    if (!ctx) return;
    pthread_mutex_lock(&ctx->lock);
    ctx->is_hidden = hidden;
    pthread_cond_signal(&ctx->cond);
    pthread_mutex_unlock(&ctx->lock);

    if (hidden && ctx->mpv && fn_mpv_set_property) {
        int paused = 1;
        fn_mpv_set_property(ctx->mpv, "pause", MPV_FORMAT_FLAG, &paused);
    }
}

void embed_player_toggle_pause(embed_context *ctx) {
    if (!ctx || !ctx->mpv || !fn_mpv_get_property || !fn_mpv_set_property) return;
    int paused = 0;
    if (fn_mpv_get_property(ctx->mpv, "pause", MPV_FORMAT_FLAG, &paused) >= 0) {
        int next = !paused;
        fn_mpv_set_property(ctx->mpv, "pause", MPV_FORMAT_FLAG, &next);
    }
}

void embed_player_set_pause(embed_context *ctx, bool paused) {
    if (!ctx || !ctx->mpv || !fn_mpv_set_property) return;
    int p = paused ? 1 : 0;
    fn_mpv_set_property(ctx->mpv, "pause", MPV_FORMAT_FLAG, &p);
}

void embed_player_seek(embed_context *ctx, double seconds, bool relative) {
    if (!ctx || !ctx->mpv || !fn_mpv_command_string) return;
    char cmd[128];
    if (relative) {
        snprintf(cmd, sizeof(cmd), "seek %f relative", seconds);
    } else {
        snprintf(cmd, sizeof(cmd), "seek %f absolute", seconds);
    }
    fn_mpv_command_string(ctx->mpv, cmd);
}

void embed_player_set_volume(embed_context *ctx, double volume) {
    if (!ctx || !ctx->mpv || !fn_mpv_set_property) return;
    if (volume < 0.0) volume = 0.0;
    if (volume > 100.0) volume = 100.0;
    fn_mpv_set_property(ctx->mpv, "volume", MPV_FORMAT_DOUBLE, &volume);
}

void embed_player_toggle_mute(embed_context *ctx) {
    if (!ctx || !ctx->mpv || !fn_mpv_get_property || !fn_mpv_set_property) return;
    int muted = 0;
    if (fn_mpv_get_property(ctx->mpv, "mute", MPV_FORMAT_FLAG, &muted) >= 0) {
        int next = !muted;
        fn_mpv_set_property(ctx->mpv, "mute", MPV_FORMAT_FLAG, &next);
    }
}

void embed_player_get_status(embed_context *ctx, embed_status *status) {
    memset(status, 0, sizeof(*status));
    if (!ctx || !ctx->mpv || !fn_mpv_get_property) return;

    status->is_playing = true;
    fn_mpv_get_property(ctx->mpv, "time-pos", MPV_FORMAT_DOUBLE, &status->time_pos);
    fn_mpv_get_property(ctx->mpv, "duration", MPV_FORMAT_DOUBLE, &status->duration);

    int paused = 0;
    if (fn_mpv_get_property(ctx->mpv, "pause", MPV_FORMAT_FLAG, &paused) >= 0) {
        status->paused = paused ? true : false;
    }

    int mute = 0;
    if (fn_mpv_get_property(ctx->mpv, "mute", MPV_FORMAT_FLAG, &mute) >= 0) {
        status->muted = mute ? true : false;
    }

    fn_mpv_get_property(ctx->mpv, "volume", MPV_FORMAT_DOUBLE, &status->volume);

    if (fn_mpv_get_property_string && fn_mpv_free) {
        char *hwdec = fn_mpv_get_property_string(ctx->mpv, "hwdec-current");
        if (hwdec) {
            strncpy(status->hwdec_current, hwdec, sizeof(status->hwdec_current) - 1);
            fn_mpv_free(hwdec);
        }
        char *vcodec = fn_mpv_get_property_string(ctx->mpv, "video-codec");
        if (!vcodec) {
            vcodec = fn_mpv_get_property_string(ctx->mpv, "video-format");
        }
        if (vcodec) {
            strncpy(status->video_codec, vcodec, sizeof(status->video_codec) - 1);
            fn_mpv_free(vcodec);
        }
    }


    int eof = 0;
    if (fn_mpv_get_property(ctx->mpv, "eof-reached", MPV_FORMAT_FLAG, &eof) >= 0) {
        status->eof_reached = eof ? true : false;
    }
}
