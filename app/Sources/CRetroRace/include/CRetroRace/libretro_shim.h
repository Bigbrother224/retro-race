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

/* Returns the pixel format the core chose: 0=0RGB1555, 1=XRGB8888, 2=RGB565. */
int rr_pixel_format(void);

/* Returns how many player controller ports the core exposes (-1 if unknown). */
int rr_controller_players(void);

/* Copies the latest captured framebuffer (in the core's pixel format) into
 * `out`. Returns 0 on success, -1 if no frame captured yet. */
int rr_snapshot(void *out, int *out_width, int *out_height);

/* Runs one frame (including video/audio callbacks). */
void rr_run(void);

/* Resets the core. */
void rr_reset(void);

/* Sets a live button state (id = RETRO_DEVICE_ID_JOYPAD_*, 0..15). */
void rr_set_button(unsigned id, int pressed);

/* Sets a live button state on a specific player port (0 or 1). */
void rr_set_button_port(unsigned port, unsigned id, int pressed);

/* Clears all button states. */
void rr_clear_buttons(void);

/* Registers the input provider. */
void rr_set_input(rr_input input);

/* Registers the video sink. Pass rr_video with on_frame == NULL to discard frames (silent ghost). */
void rr_set_video(rr_video video);

/* Sets where the core reads/writes its system and save files. Must be called
 * before rr_load (the core queries these during init). */
void rr_set_system_dir(const char *dir);
void rr_set_save_dir(const char *dir);

/* Save-state support. Returns serialize size. */
size_t rr_serialize_size(void);
int rr_serialize(void *data, size_t size);
int rr_unserialize(const void *data, size_t size);

#ifdef __cplusplus
}
#endif

#endif /* LIBRETRO_SHIM_H */
