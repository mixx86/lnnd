Service=mas alla de map simple con ops simples como crud, decisiones condicionadas, logica negocio...
Manager=simple ops, simple map, simple crud

# Scheduler

// definir en comentarios el orden de prioridades para qid (queue id) y bpid (backpressure id) para los dos subsistemas pensados actualmente (requests y links)

  type Sched struct {
      queues    map[QueueID]queue.Queue
      pressure  map[BPSourceID]*atomic.Uint32
      threshold uint64
  }

En cuanto a ram:

definir constantes de prioridad de subsistemas

type Queue[T any] interface {
  Enqueue(item T) error
  TryDequeue() (T, bool)
}

Condiciones escribir en Requests:
  - RAM files no al 100%
  - Request lleva mas de x tiempo esperando (anti-starvation)

En cuanto a disco:

sched apunta los backpressures en orden de prioridad de mayor a menor

si hay backpressure se va desbloqueando en cascada, de mayor prioridad a menor

ejemplo detectar backpressure en un subsistema:
start := time.Now()
// recibe completion
latency := time.Since(start)
if latency > threshold {
    reportPressure(PressureHigh)
}

configurar threshold (máximo de uso de memoria de lnnd para operaciones) en los env file
como LNND_MAX_MEM=64MB

cabe destacar que debemos testear el valor
