#include "include/CRetroRace/rr_shm.h"

#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

/*
 * Seqlock discipline:
 *   producer: write payload -> __sync_synchronize -> frame++
 *   reader:   read frame into f1 -> read payload -> sync -> read frame into f2
 *             accept iff f1 == f2 && f1 != *lastFrame
 */
static uint32_t load32(volatile uint32_t *p) {
    return __atomic_load_n(p, __ATOMIC_ACQUIRE);
}

int rr_shm_open_named(const char *name, bool publish, rr_shm *out) {
    if (!name || !out) {
        return -1;
    }
    memset(out, 0, sizeof(*out));

    snprintf(out->name, sizeof(out->name), "/%s", name);
    int oflags = publish ? (O_CREAT | O_RDWR) : O_RDWR;
    mode_t mode = 0600;

    /* A producer wants a fresh region. Unlink first so a stale region from a
     * previous run (possibly still mapped by a dead consumer) does not make
     * shm_open/ftruncate fail with EINVAL. */
    if (publish) {
        shm_unlink(out->name);
    }

    int fd = shm_open(out->name, oflags, mode);
    if (fd < 0) {
        fprintf(stderr, "rr_shm_open_named: shm_open(%s) failed: %s\n", out->name, strerror(errno));
        return -1;
    }

    /* Producer sizes the region; a reader only maps what already exists. */
    if (publish && ftruncate(fd, (off_t)RR_SHM_PAYLOAD_SIZE) != 0) {
        fprintf(stderr, "rr_shm_open_named: ftruncate failed: %s\n", strerror(errno));
        close(fd);
        return -1;
    }

    void *map = mmap(NULL, RR_SHM_PAYLOAD_SIZE,
                     PROT_READ | PROT_WRITE,
                     MAP_SHARED, fd, 0);
    if (map == MAP_FAILED) {
        fprintf(stderr, "rr_shm_open_named: mmap failed: %s\n", strerror(errno));
        close(fd);
        return -1;
    }

    out->map = map;
    out->len = RR_SHM_PAYLOAD_SIZE;
    out->fd = fd;
    return 0;
}

void rr_shm_close(rr_shm *shm, bool owner) {
    if (!shm || !shm->map) {
        return;
    }
    munmap(shm->map, shm->len);
    shm->map = NULL;
    if (shm->fd >= 0) {
        close(shm->fd);
        shm->fd = -1;
    }
    if (owner) {
        shm_unlink(shm->name);
    }
}

void rr_shm_publish(rr_shm *shm, const rr_shm_slot *slot) {
    if (!shm || !shm->map || !slot) {
        return;
    }
    rr_shm_slot *dst = (rr_shm_slot *)shm->map;
    /* Copy the whole slot (position, tile, ...), then publish the frame
     * number with a release so the reader never observes a torn slot. */
    memcpy(dst, slot, sizeof(rr_shm_slot));
    __sync_synchronize();
    __atomic_store_n(&dst->frame, slot->frame, __ATOMIC_RELEASE);
}

bool rr_shm_take(rr_shm *shm, uint32_t *lastFrame, rr_shm_slot *out) {
    if (!shm || !shm->map || !out || !lastFrame) {
        return false;
    }
    const rr_shm_slot *src = (const rr_shm_slot *)shm->map;

    uint32_t f1 = load32(&src->frame);
    uint32_t f2 = f1;
    do {
        memcpy(out, src, sizeof(rr_shm_slot));
        f2 = load32(&src->frame);
    } while (f1 != f2);

    if (f1 == *lastFrame) {
        return false; /* no new frame */
    }
    *lastFrame = f1;
    return out->magic == RR_SHM_MAGIC;
}

void rr_shm_slot_set_tile(rr_shm_slot *slot, const uint8_t *tile, uint32_t count) {
    if (!slot || !tile) {
        return;
    }
    if (count > RR_SHM_TILE_BYTES) {
        count = RR_SHM_TILE_BYTES;
    }
    memcpy(slot->tile, tile, count);
}

void rr_shm_slot_tile_copy(const rr_shm_slot *slot, uint8_t *out, uint32_t count) {
    if (!slot || !out) {
        return;
    }
    if (count > RR_SHM_TILE_BYTES) {
        count = RR_SHM_TILE_BYTES;
    }
    memcpy(out, slot->tile, count);
}

uint64_t rr_shm_slot_tile_sum(const rr_shm_slot *slot) {
    if (!slot) {
        return 0;
    }
    uint64_t sum = 0;
    for (uint32_t i = 0; i < RR_SHM_TILE_BYTES; i++) {
        sum += slot->tile[i];
    }
    return sum;
}