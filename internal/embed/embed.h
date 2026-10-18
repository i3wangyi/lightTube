#ifndef LIGHTTUBE_EMBED_H
#define LIGHTTUBE_EMBED_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct embed_context embed_context;

embed_context *embed_context_create(void *wl_display, void *wl_surface);
void embed_context_destroy(embed_context *ctx);

int embed_player_start(embed_context *ctx, const char *url, const char *title,
                       const char *ytdl_format, const char *cookies_file,
                       int width_px, int height_px, int scale, int debug);
void embed_player_stop(embed_context *ctx);

void embed_player_set_rect(embed_context *ctx, int x_px, int y_px, int w_px, int h_px, int scale);
void embed_player_set_hidden(embed_context *ctx, bool hidden);

void embed_player_toggle_pause(embed_context *ctx);
void embed_player_set_pause(embed_context *ctx, bool paused);
void embed_player_seek(embed_context *ctx, double seconds, bool relative);
void embed_player_set_volume(embed_context *ctx, double volume);
void embed_player_toggle_mute(embed_context *ctx);

typedef struct embed_status {
    double time_pos;
    double duration;
    bool paused;
    bool muted;
    double volume;
    char hwdec_current[32];
    bool is_playing;
    bool eof_reached;
} embed_status;

void embed_player_get_status(embed_context *ctx, embed_status *status);

#ifdef __cplusplus
}
#endif

#endif /* LIGHTTUBE_EMBED_H */
