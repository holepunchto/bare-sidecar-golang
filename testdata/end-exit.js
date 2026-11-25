Bare.IPC.on('close', () => {
  setTimeout(() => Bare.exit(3), 100)
}).end()
