#ifndef LIGHTTUBE_X11_H
#define LIGHTTUBE_X11_H

#include <stdint.h>
#include <stdbool.h>
#include "embed.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct x11_embed_context x11_embed_context;

x11_embed_context *x11_embed_context_create(unsigned long parent_window);
void x11_embed_context_destroy(x11_embed_context *ctx);

int x11_embed_player_start(x11_embed_context *ctx, const char *url, const char *title,
                           const char *ytdl_format, const char *cookies_file,
                           const char *hwdec,
                           int width_px, int height_px, int scale, int debug);
void x11_embed_player_stop(x11_embed_context *ctx);

void x11_embed_player_set_rect(x11_embed_context *ctx, int x_px, int y_px, int w_px, int h_px, int scale);
void x11_embed_player_set_hidden(x11_embed_context *ctx, bool hidden);

void x11_embed_player_toggle_pause(x11_embed_context *ctx);
void x11_embed_player_set_pause(x11_embed_context *ctx, bool paused);
void x11_embed_player_seek(x11_embed_context *ctx, double seconds, bool relative);
void x11_embed_player_set_volume(x11_embed_context *ctx, double volume);
void x11_embed_player_toggle_mute(x11_embed_context *ctx);

void x11_embed_player_get_status(x11_embed_context *ctx, embed_status *status);

#ifdef __cplusplus
}
#endif

#endif /* LIGHTTUBE_X11_H */
