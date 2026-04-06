#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

int ipc_connect(void) {
  struct sockaddr_un addr;
  int soc = socket(AF_UNIX, SOCK_STREAM, 0);
  if (soc < 0) {
    fprintf(stderr, "Error creating the socket.\n");
    return -1;
  }
  addr.sun_family = AF_UNIX;
  strncpy(addr.sun_path, "/tmp/lnnd.sock", sizeof(addr.sun_path));
  addr.sun_path[sizeof(addr.sun_path) - 1] = '\0';

  if (connect(soc, (struct sockaddr *)&addr, sizeof(struct sockaddr_un)) < 0) {
    close(soc);
    fprintf(stderr, "Error establishing the connection.\n");
    return -1;
  }
  return soc;
}

int ipc_disconnect(int soc) {
  close(soc);
  return 0;
}

int ipc_send(int soc, void *buff, size_t len) {
  // TODO: Implement write_full
  if (write(soc, buff, len) < 0) {
    fprintf(stderr, "Error while trying to send data.\n");
    return -1;
  }
  return 0;
}

int ipc_recv(int soc, void *buff, size_t len) {
  ssize_t n = read(soc, buff, len);
  if (n < 0) {
    fprintf(stderr, "Error while trying to read data.\n");
    return -1;
  }
  return n;
}
