#ifndef HWCAPS_H
#define HWCAPS_H

#include <stddef.h>

typedef struct {
    int h264;
    int hevc;
    int vp9;
    int av1;
    char driver[64];
} va_decode_caps_t;

int query_decode_caps(va_decode_caps_t *caps, char *err_buf, size_t err_len);
int check_libmpv_loadable(char *err_buf, size_t err_len);

#endif // HWCAPS_H
