// SPIKE: runs the demo in a simulator process, taps through it, prints what it sees.
#include <stdio.h>
#include <string.h>
#include <unistd.h>
#include <pthread.h>
#include "libmbdemo.h"

static char seen[4 * 1024 * 1024];
static size_t n;
static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;

static void *reader(void *arg) {
    static char buf[65536];
    for (;;) {
        int got = MBDemoRead(buf, sizeof buf);
        if (got < 0) return NULL;
        pthread_mutex_lock(&mu);
        if (n + got < sizeof seen) { memcpy(seen + n, buf, got); n += got; seen[n] = 0; }
        pthread_mutex_unlock(&mu);
    }
}

static int has(const char *s) {
    pthread_mutex_lock(&mu);
    int ok = strstr(seen, s) != NULL;
    pthread_mutex_unlock(&mu);
    return ok;
}

static int wait_for(const char *s, int secs) {
    for (int i = 0; i < secs * 10; i++) { if (has(s)) return 1; usleep(100000); }
    return 0;
}

static void keys(const char *s) { MBDemoWrite((char *)s, (int)strlen(s)); }

int main(void) {
    pthread_t t;
    MBDemoStart(44, 31);
    pthread_create(&t, NULL, reader, NULL);
    int list = wait_for("flint-auth", 15);
    printf("RESULT list drawn: %d\n", list);
    keys("\r");
    sleep(2);
    keys("2");
    int invite = wait_for("matchblox.sh", 15);
    printf("RESULT bytes drawn: %zu\n", n);
    printf("RESULT invite drawn: %d\n", invite);
    return list && invite ? 0 : 1;
}
