#include "include/CRetroRace/libretro_shim.h"

#include <dlfcn.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ---- libretro.h essentials (subset) ---- */

typedef bool (*retro_environment_t)(unsigned cmd, void *data);
typedef void (*retro_video_refresh_t)(const void *data, unsigned width, unsigned height, size_t pitch);
typedef void (*retro_audio_sample_t)(int16_t left, int16_t right);
typedef size_t (*retro_audio_sample_batch_t)(const int16_t *data, size_t frames);
typedef void (*retro_input_poll_t)(void);
typedef int16_t (*retro_input_state_t)(unsigned port, unsigned device, unsigned index, unsigned id);

struct retro_system_info {
    const char *library_name;
    const char *library_version;
    const char *valid_extensions;
    bool need_fullpath;
    bool block_extract;
};

struct retro_game_geometry {
    unsigned base_width;
    unsigned base_height;
    unsigned max_width;
    unsigned max_height;
    float aspect_ratio;
};

struct retro_system_timing {
    double fps;
    double sample_rate;
};

struct retro_system_av_info {
    struct retro_game_geometry geometry;
    struct retro_system_timing timing;
};

struct retro_game_info {
    const char *path;
    const void *data;
    size_t size;
    const char *meta;
};

enum {
    RETRO_ENVIRONMENT_GET_SYSTEM_DIRECTORY = 9,
    RETRO_ENVIRONMENT_SET_PIXEL_FORMAT = 10,
    RETRO_ENVIRONMENT_GET_VARIABLE = 15,
    RETRO_ENVIRONMENT_SET_VARIABLES = 16,
    RETRO_ENVIRONMENT_GET_VARIABLE_UPDATE = 17,
    RETRO_ENVIRONMENT_GET_LOG_INTERFACE = 27,
    RETRO_ENVIRONMENT_GET_SAVE_DIRECTORY = 31,
    RETRO_ENVIRONMENT_SET_CONTROLLER_INFO = 35,
    RETRO_ENVIRONMENT_SET_GEOMETRY = 37
};

enum {
    RETRO_PIXEL_FORMAT_0RGB1555 = 0,
    RETRO_PIXEL_FORMAT_XRGB8888 = 1,
    RETRO_PIXEL_FORMAT_RGB565 = 2
};

/* ---- setter functions exported by the core ---- */

static void (*g_set_environment)(retro_environment_t);
static void (*g_set_video_refresh)(retro_video_refresh_t);
static void (*g_set_audio_sample)(retro_audio_sample_t);
static void (*g_set_audio_sample_batch)(retro_audio_sample_batch_t);
static void (*g_set_input_poll)(retro_input_poll_t);
static void (*g_set_input_state)(retro_input_state_t);

/* ---- core lifecycle symbols ---- */

static void *g_core;
static void (*g_init)(void);
static void (*g_deinit)(void);
static unsigned (*g_api_version)(void);
static void (*g_get_system_info)(struct retro_system_info *);
static void (*g_get_system_av_info)(struct retro_system_av_info *);
static void (*g_set_controller_port_device)(unsigned, unsigned);
static void (*g_reset)(void);
static void (*g_run)(void);
static size_t (*g_serialize_size)(void);
static bool (*g_serialize)(void *, size_t);
static bool (*g_unserialize)(const void *, size_t);
static bool (*g_load_game)(const struct retro_game_info *);
static void (*g_unload_game)(void);

static rr_input g_input = {0};
static rr_video g_video_sink = {0};
static const char *g_system_dir = "/tmp";
static const char *g_save_dir = "/tmp";
static bool g_need_fullpath = false;

static bool env_cb(unsigned cmd, void *data) {
    switch (cmd) {
        case RETRO_ENVIRONMENT_GET_SYSTEM_DIRECTORY:
            *(const char **)data = g_system_dir;
            return true;
        case RETRO_ENVIRONMENT_GET_SAVE_DIRECTORY:
            *(const char **)data = g_save_dir;
            return true;
        case RETRO_ENVIRONMENT_SET_PIXEL_FORMAT: {
            /* We accept whatever the core picks (RGB565 or XRGB8888). */
            int fmt = *(int *)data;
            (void)fmt;
            return true;
        }
        default:
            return false;
    }
}

static void input_poll_cb(void) {
    if (g_input.poll) {
        g_input.poll(g_input.user);
    }
}

static int16_t input_state_cb(unsigned port, unsigned device, unsigned index, unsigned id) {
    if (g_input.state) {
        return g_input.state(port, device, index, id, g_input.user);
    }
    return 0;
}

static void video_cb(const void *data, unsigned width, unsigned height, size_t pitch) {
    if (!data || !g_video_sink.on_frame) {
        return;
    }
    rr_frame f;
    f.width = (int)width;
    f.height = (int)height;
    f.pitch = pitch;
    f.framebuffer = data;
    g_video_sink.on_frame(f, g_video_sink.user);
}

static void audio_cb(int16_t l, int16_t r) {
    (void)l;
    (void)r;
}

static size_t audio_batch_cb(const int16_t *data, size_t frames) {
    (void)data;
    return frames;
}

int rr_load(const char *core_path) {
    if (g_core) {
        rr_unload();
    }

    g_core = dlopen(core_path, RTLD_NOW | RTLD_LOCAL);
    if (!g_core) {
        fprintf(stderr, "rr_load: dlopen failed: %s\n", dlerror());
        return -1;
    }

#define SET(name)                                                                                  \
    do {                                                                                           \
        g_set_##name = (typeof(g_set_##name))dlsym(g_core, "retro_set_" #name);                    \
        if (!g_set_##name) {                                                                       \
            fprintf(stderr, "rr_load: missing retro_set_%s: %s\n", #name, dlerror());              \
            dlclose(g_core);                                                                       \
            g_core = NULL;                                                                         \
            return -2;                                                                             \
        }                                                                                          \
    } while (0)

    SET(environment);
    SET(video_refresh);
    SET(audio_sample);
    SET(audio_sample_batch);
    SET(input_poll);
    SET(input_state);

#undef SET

#define SYM(name)                                                                                  \
    do {                                                                                           \
        g_##name = (typeof(g_##name))dlsym(g_core, "retro_" #name);                                \
        if (!g_##name) {                                                                           \
            fprintf(stderr, "rr_load: missing retro_%s: %s\n", #name, dlerror());                  \
            dlclose(g_core);                                                                       \
            g_core = NULL;                                                                         \
            return -2;                                                                             \
        }                                                                                          \
    } while (0)

    SYM(init);
    SYM(deinit);
    SYM(api_version);
    SYM(get_system_info);
    SYM(get_system_av_info);
    SYM(set_controller_port_device);
    SYM(reset);
    SYM(run);
    SYM(serialize_size);
    SYM(serialize);
    SYM(unserialize);
    SYM(load_game);
    SYM(unload_game);

#undef SYM

    return 0;
}

void rr_unload(void) {
    if (!g_core) {
        return;
    }
    if (g_deinit) {
        g_deinit();
    }
    dlclose(g_core);
    g_core = NULL;
}

unsigned rr_api_version(void) {
    return g_api_version ? g_api_version() : 0;
}

int rr_system_info(char *out, size_t out_size) {
    if (!g_get_system_info) {
        return -1;
    }
    struct retro_system_info info;
    memset(&info, 0, sizeof(info));
    g_get_system_info(&info);
    g_need_fullpath = info.need_fullpath;
    if (out && out_size) {
        snprintf(out, out_size, "%s %s (%s)", info.library_name ? info.library_name : "?",
                 info.library_version ? info.library_version : "?", info.valid_extensions ? info.valid_extensions : "?");
    }
    return 0;
}

int rr_load_game(const void *rom, size_t rom_size) {
    if (!g_load_game) {
        return -1;
    }

    g_set_environment(env_cb);
    g_set_video_refresh(video_cb);
    g_set_audio_sample(audio_cb);
    g_set_audio_sample_batch(audio_batch_cb);
    g_set_input_poll(input_poll_cb);
    g_set_input_state(input_state_cb);

    struct retro_game_info info;
    memset(&info, 0, sizeof(info));

    if (g_need_fullpath) {
        /* Some cores (e.g. FCEUmm) require a real file path. Write the ROM to
         * a temp file and pass the path, so callers can keep a buffer API. */
        const char *tmp = "/tmp/retro_race_game.nes";
        FILE *f = fopen(tmp, "wb");
        if (!f) {
            return -3;
        }
        size_t written = fwrite(rom, 1, rom_size, f);
        fclose(f);
        if (written != rom_size) {
            return -3;
        }
        info.path = tmp;
    } else {
        info.path = "game.nes";
        info.data = rom;
        info.size = rom_size;
    }

    g_init();
    g_set_controller_port_device(0, 1 /* RETRO_DEVICE_JOYPAD */);
    if (!g_load_game(&info)) {
        return -2;
    }
    return 0;
}

void rr_unload_game(void) {
    if (g_unload_game) {
        g_unload_game();
    }
    if (g_deinit) {
        g_deinit();
    }
}

void rr_av_info(unsigned *base_width, unsigned *base_height, double *fps, unsigned *max_width, unsigned *max_height) {
    struct retro_system_av_info av;
    memset(&av, 0, sizeof(av));
    g_get_system_av_info(&av);
    if (base_width) {
        *base_width = av.geometry.base_width;
    }
    if (base_height) {
        *base_height = av.geometry.base_height;
    }
    if (fps) {
        *fps = av.timing.fps;
    }
    if (max_width) {
        *max_width = av.geometry.max_width;
    }
    if (max_height) {
        *max_height = av.geometry.max_height;
    }
}

void rr_run(void) {
    g_run();
}

void rr_reset(void) {
    g_reset();
}

void rr_set_input(rr_input input) {
    g_input = input;
}

void rr_set_video(rr_video video) {
    g_video_sink = video;
}

size_t rr_serialize_size(void) {
    return g_serialize_size();
}

int rr_serialize(void *data, size_t size) {
    return g_serialize(data, size) ? 0 : -1;
}

int rr_unserialize(const void *data, size_t size) {
    return g_unserialize(data, size) ? 0 : -1;
}
