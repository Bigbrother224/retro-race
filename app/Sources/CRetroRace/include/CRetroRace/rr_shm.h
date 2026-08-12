#ifndef RETRO_RACE_SHM_H
#define RETRO_RACE_SHM_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Shared-memory channel between the player process (consumer) and the silent
 * ghost process (producer). Layout is fixed so both binaries can be rebuilt
 * independently: a versioned header followed by a roasted sprite region.
 *
 * A "slot" is one published frame of ghost state. A seqlock-style frame
 * counter lets the reader detect torn reads.
 */

#define RR_SHM_MAGIC   0x52525348u      /* "RRSH" */
#define RR_SHM_VERSION 1u

#define RR_SHM_TILE_W 32u
#define RR_SHM_TILE_H 32u
#define RR_SHM_TILE_BYTES (RR_SHM_TILE_W * RR_SHM_TILE_H * 4u)

/* Total mapping: header + tile. Fixed so the consumer can map without
 * reading the size from the producer. */
#define RR_SHM_PAYLOAD_SIZE (128u + RR_SHM_TILE_BYTES)

typedef struct {
    uint32_t magic;
    uint32_t version;
    uint32_t frame;       /* increments every publish; reader uses it as seqlock */
    uint32_t state;       /* RR_SHM_STATE_* below */
    uint32_t flags;
    uint32_t game_fps;

    /* calibrated position of the ghost character, in game pixels */
    uint32_t pos_x;
    uint32_t pos_y;

    /* reserved for future use; keeps the record stable across binaries */
    uint32_t reserved[4];

    uint8_t tile[RR_SHM_TILE_BYTES];   /* RGBA, tightly packed */
} rr_shm_slot;

enum {
    RR_SHM_STATE_IDLE     = 0,  /* published but not racing yet */
    RR_SHM_STATE_RACING   = 1,  /* ghost is live */
    RR_SHM_STATE_DONE     = 2,  /* segment finished */
    RR_SHM_STATE_ABORTED  = 3,  /* cancelled */
};

typedef struct {
    void *map;
    size_t len;
    int fd;
    char name[64];
} rr_shm;

/* Opens (creates if needed, when publish=1) the shm region named `name`
 * (with a leading '/' added). Returns 0 on success. */
int rr_shm_open_named(const char *name, bool publish, rr_shm *out);

/* Unmaps and (if owner) unlinks the region. */
void rr_shm_close(rr_shm *shm, bool owner);

/* Publishes one frame. Fills the slot and bumps `frame` after the payload is
 * fully written so the reader never sees a half-written slot. */
void rr_shm_publish(rr_shm *shm, const rr_shm_slot *slot);

/* Tries to read the latest slot. Returns true if a new frame was captured
 * whole (frame advanced since *lastFrame); false if no new frame or torn. */
bool rr_shm_take(rr_shm *shm, uint32_t *lastFrame, rr_shm_slot *out);

/* Copies `count` bytes into the slot's tile region (call before publish).
 * Avoids exposing the 4096-byte C array directly to Swift. */
void rr_shm_slot_set_tile(rr_shm_slot *slot, const uint8_t *tile, uint32_t count);

/* Copies `count` bytes from the slot's tile region into `out`. */
void rr_shm_slot_tile_copy(const rr_shm_slot *slot, uint8_t *out, uint32_t count);

/* Sums the tile bytes (used as a cheap non-zero/motion hash by the reader). */
uint64_t rr_shm_slot_tile_sum(const rr_shm_slot *slot);

#ifdef __cplusplus
}
#endif

#endif /* RETRO_RACE_SHM_H */