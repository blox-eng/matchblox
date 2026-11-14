// Runs the demo in a simulator process, turns from the board to Machine and back, and prints
// what it saw.
#include <pthread.h>
#include <stdio.h>
#include <string.h>
#include <sys/resource.h>
#include <unistd.h>

#include "libmbdemo.h"

static char seen[4 * 1024 * 1024];
static size_t seen_len, drawn;
static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;

static void *reader(void *arg) {
    static char buf[65536];
    (void)arg;
    for (;;) {
        int got = MBDemoRead(buf, sizeof buf);
        if (got < 0) return NULL;
        pthread_mutex_lock(&mu);
        drawn += (size_t)got;
        if (seen_len + (size_t)got < sizeof seen) {
            memcpy(seen + seen_len, buf, (size_t)got);
            seen_len += (size_t)got;
            seen[seen_len] = 0;
        }
        pthread_mutex_unlock(&mu);
    }
}

static int wait_for(const char *s, int secs) {
    for (int i = 0; i < secs * 10; i++) {
        pthread_mutex_lock(&mu);
        int ok = strstr(seen, s) != NULL;
        pthread_mutex_unlock(&mu);
        if (ok) return 1;
        usleep(100000);
    }
    return 0;
}

// press sends a key and waits for s among what is drawn after it.
static int press(const char *key, const char *s, int secs) {
    pthread_mutex_lock(&mu);
    seen_len = 0;
    seen[0] = 0;
    pthread_mutex_unlock(&mu);
    MBDemoWrite((char *)key, (int)strlen(key));
    return wait_for(s, secs);
}

int main(int argc, char **argv) {
    setvbuf(stdout, NULL, _IOLBF, 0); // what it saw survives a crash
    if (argc != 2) {
        fprintf(stderr, "usage: probe <tree>\n");
        return 2;
    }
    int code = MBDemoStart(argv[1], 44, 31);
    if (code != 0) {
        char why[1024];
        MBDemoError(why, sizeof why);
        printf("RESULT start failed: %d: %s\n", code, why);
        return 1;
    }
    MBDemoWrite(NULL, 1);
    printf("RESULT a NULL read and write return: %d\n", MBDemoRead(NULL, 1) == -1);
    pthread_t t;
    pthread_create(&t, NULL, reader, NULL);

    // The board opens on NEEDS YOU, flint-auth first; Tab turns to Machine and back (a digit
    // on the board answers its card).
    int board = wait_for("flint-auth", 30);
    printf("RESULT the board drew flint-auth: %d\n", board);
    int tab = board && press("\t", "load 6.2", 15);
    printf("RESULT tab drew Machine: %d\n", tab);
    int machine = tab && wait_for("16 cores", 15);
    printf("RESULT the machine's 16 cores drawn: %d\n", machine);
    int back = tab && press("\t", "flint-auth", 15);
    printf("RESULT tab drew the board again: %d\n", back);

    struct rusage use;
    getrusage(RUSAGE_SELF, &use);
    pthread_mutex_lock(&mu);
    printf("RESULT bytes drawn: %zu\n", drawn);
    pthread_mutex_unlock(&mu);
    // ru_maxrss is bytes on Darwin, and kilobytes on Linux.
    printf("RESULT resident peak: %ld ru_maxrss\n", (long)use.ru_maxrss);

    MBDemoStop();
    pthread_join(t, NULL);
    printf("RESULT stopped: 1\n");
    return board && tab && machine && back ? 0 : 1;
}
