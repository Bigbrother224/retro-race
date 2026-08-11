#ifndef LIBRETRO_SHIM_H
#define LIBRETRO_SHIM_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    int width;
    int height;
    size_t pitch;
    const void *framebuffer;
} rr_frame;

typedef struct {
    void (*poll)(void *user);
    int16_t (*state)(unsigned port, unsigned device, unsigned index, unsigned id, void *user);
    void *user;
} rr_input;

typedef struct {
    void (*on_frame)(rr_frame frame, void *user);
    void *user;
} rr_video;

/* Loads a libretro core (.dylib) and binds the low-level API. Returns 0 on success. */
int rr_load(const char *core_path);

/* Unloads the core. */
void rr_unload(void);

/* Returns the core's API version. */
unsigned rr_api_version(void);

/* Queries system info (short name, library name, extensions). */
int rr_system_info(char *out, size_t out_size);

/* Loads a game from an in-memory buffer. Returns 0 on success. */
int rr_load_game(const void *rom, size_t rom_size);

/* Unloads the game. */
void rr_unload_game(void);

/* Returns base video geometry. */
void rr_av_info(unsigned *base_width, unsigned *base_height, double *fps, unsigned *max_width, unsigned *max_height);

/* Runs one frame (including video/audio callbacks). */
void rr_run(void);

/* Resets the core. */
void rr_reset(void);

/* Registers the input provider. */
void rr_set_input(rr_input input);

/* Registers the video sink. Pass rr_video with on_frame == NULL to discard frames (silent ghost). */
void rr_set_video(rr_video video);

/* Save-state support. Returns serialize size. */
size_t rr_serialize_size(void);
int rr_serialize(void *data, size_t size);
int rr_unserialize(const void *data, size_t size);

#ifdef __cplusplus
}
#endif

#endif /* LIBRETRO_SHIM_H */
