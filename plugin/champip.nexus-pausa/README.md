# Nexus · Pausa activa

Overlay de pantalla completa para los recordatorios de movimiento de Nexus. Se muestra por encima de las ventanas en todas las pantallas, recibe el mensaje, el consejo y la duración de cada pausa, y permite posponerla o saltarla.

## Prueba manual

Con el plugin enlazado y habilitado en Omarchy, invócalo desde una terminal:

```sh
omarchy-shell shell summon champip.nexus-pausa '{"message":"Levántate y camina un poco.","tip":"Estira suavemente las piernas.","seconds":30}'
```

Los segundos ausentes o inválidos usan 30; el valor se limita al intervalo de 5 a 300 segundos.

## Contrato de respuesta

Al terminar la cuenta regresiva, el overlay ejecuta `nexus pausa hecho`. Los botones ejecutan `nexus pausa posponer` (10 minutos) y `nexus pausa saltar`, respectivamente. Los comandos se inician con argumentos separados, no mediante una cadena de shell. El overlay oculta la superficie después de enviar la respuesta.
