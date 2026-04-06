#ifndef IPC_H
#define IPC_H

#include <stddef.h>

int ipc_connect(void);
int ipc_disconnect(int soc);
int ipc_send(int soc, void *buff, size_t len);
int ipc_recv(int soc, void *buff, size_t len);

#endif
