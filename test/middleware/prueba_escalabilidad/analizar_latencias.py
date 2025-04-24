import csv
import numpy as np
import re
from collections import defaultdict

# Nombre del archivo original
archivo_origen = "datos_escalabilidad_20_20_middleware.csv"

# Extraer x e y del nombre del archivo original
match = re.search(r"(\d+)_(\d+)", archivo_origen)
if match:
    x, y = match.groups()
else:
    raise ValueError("No se encontraron números en el nombre del archivo original.")

# Nombre del archivo de resultados
archivo_resultados = f"resultados_{x}_{y}.csv"

# Leer el contenido del archivo original y agrupar por emisor y receptor
latencias_por_grupo = defaultdict(list)

with open(archivo_origen, mode='r') as archivo_csv:
    lector_csv = csv.reader(archivo_csv)
    for fila in lector_csv:
        tiempo_envio = int(fila[0])
        tiempo_recepcion = int(fila[1])
        latencia = int(fila[2])
        emisor = fila[3]
        receptor = fila[4]
        latencias_por_grupo[(emisor, receptor)].append(latencia)

# Escribir los resultados en el nuevo archivo
with open(archivo_resultados, mode='w', newline='') as archivo_csv_resultados:
    escritor_csv = csv.writer(archivo_csv_resultados)
    # Escribir encabezados
    escritor_csv.writerow([
        "Publicador", "Suscriptor", "Mensajes", "Latencia Promedio (ms)", 
        "Mediana (ms)", "P95 (ms)", "P99 (ms)", 
        "Latencia Mínima (ms)", "Latencia Máxima (ms)", "Desviación Estándar (ms)"
    ])
    
    # Calcular métricas para cada grupo y escribirlas
    for (emisor, receptor), latencias in latencias_por_grupo.items():
        latencias_ms = np.array(latencias) / 1_000_000  # Convertir nanosegundos a milisegundos
        latencia_promedio = np.mean(latencias_ms)
        mediana = np.median(latencias_ms)
        p95 = np.percentile(latencias_ms, 95)
        p99 = np.percentile(latencias_ms, 99)
        latencia_minima = np.min(latencias_ms)
        latencia_maxima = np.max(latencias_ms)
        desviacion_estandar = np.std(latencias_ms)
        
        escritor_csv.writerow([
            emisor, receptor, len(latencias), 
            round(latencia_promedio, 2), round(mediana, 2), round(p95, 2), round(p99, 2), 
            round(latencia_minima, 2), round(latencia_maxima, 2), round(desviacion_estandar, 2)
        ])

print(f"Archivo generado: {archivo_resultados}")