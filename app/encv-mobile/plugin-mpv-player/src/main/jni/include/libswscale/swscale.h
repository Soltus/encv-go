#pragma once
enum AVPixelFormat {
    AV_PIX_FMT_NONE = -1,
    AV_PIX_FMT_RGB32 = 25,
    AV_PIX_FMT_BGR0 = 28,
};
#define SWS_BICUBIC 4
struct SwsContext;
#ifdef __cplusplus
extern "C" {
#endif
struct SwsContext *sws_getContext(int srcW, int srcH, int srcFormat,
    int dstW, int dstH, int dstFormat, int flags,
    void *srcFilter, void *dstFilter, const double *params);
int sws_scale(struct SwsContext *c, const uint8_t *const srcSlice[], const int srcStride[],
    int srcSliceY, int srcSliceH, uint8_t *const dst[], const int dstStride[]);
void sws_freeContext(struct SwsContext *swsContext);
#ifdef __cplusplus
}
#endif
