
import pandas as pd
import os
import glob
import re

def procesar_csvs(directorio):
    archivos = glob.glob(os.path.join(directorio, "datos_*csv"))

    resumen_middleware = []
    resumen_solo = []

    for archivo in archivos:
        df = pd.read_csv(archivo, header=None, names=["enviado", "recibido", "latencia"])
        archivo_base = os.path.basename(archivo)

        if "solo" in archivo_base:
            protocolo = re.search(r"datos_(.*?)_solo\.csv", archivo_base).group(1).upper()
            resumen_solo.append({
                "Protocolo": protocolo,
                "Mensajes": len(df),
                "Latencia Promedio (ms)": round(df["latencia"].mean() / 1e6, 2),
                "Mediana (ms)": round(df["latencia"].median() / 1e6, 2),
                "P95 (ms)": round(df["latencia"].quantile(0.95) / 1e6, 2),
                "P99 (ms)": round(df["latencia"].quantile(0.99) / 1e6, 2),
                "Latencia Mínima (ms)": round(df["latencia"].min() / 1e6, 2),
                "Latencia Máxima (ms)": round(df["latencia"].max() / 1e6, 2),
                "Desviación Estándar (ms)": round(df["latencia"].std() / 1e6, 2)
            })
        else:
            match = re.search(r"datos_(.*?)_middleware\.csv", archivo_base)
            pub, sub = match.group(1).split("_")
            resumen_middleware.append({
                "Publicador": pub.upper(),
                "Suscriptor": sub.upper(),
                "Mensajes": len(df),
                "Latencia Promedio (ms)": round(df["latencia"].mean() / 1e6, 2),
                "Mediana (ms)": round(df["latencia"].median() / 1e6, 2),
                "P95 (ms)": round(df["latencia"].quantile(0.95) / 1e6, 2),
                "P99 (ms)": round(df["latencia"].quantile(0.99) / 1e6, 2),
                "Latencia Mínima (ms)": round(df["latencia"].min() / 1e6, 2),
                "Latencia Máxima (ms)": round(df["latencia"].max() / 1e6, 2),
                "Desviación Estándar (ms)": round(df["latencia"].std() / 1e6, 2)
            })

    df_middleware = pd.DataFrame(resumen_middleware)
    df_solo = pd.DataFrame(resumen_solo)

    # Calcular overhead para combinaciones homogéneas
    overhead = []
    for _, row in df_middleware.iterrows():
        if row["Publicador"] == row["Suscriptor"]:
            proto = row["Publicador"]
            lat_solo = df_solo[df_solo["Protocolo"] == proto]["Latencia Promedio (ms)"].values
            if lat_solo.size > 0:
                overhead.append({
                    "Protocolo": proto,
                    "Latencia Sin Middleware (ms)": lat_solo[0],
                    "Latencia Con Middleware (ms)": row["Latencia Promedio (ms)"],
                    "Overhead Introducido (ms)": round(row["Latencia Promedio (ms)"] - lat_solo[0], 2)
                })

    df_overhead = pd.DataFrame(overhead)

    # Guardar como CSV
    df_middleware.to_csv(os.path.join(directorio, "resumen_middleware.csv"), index=False)
    df_solo.to_csv(os.path.join(directorio, "resumen_solo.csv"), index=False)
    df_overhead.to_csv(os.path.join(directorio, "resumen_overhead.csv"), index=False)

    print("\n🧩 Resultados con Middleware guardados en resumen_middleware.csv")
    print("\n🚀 Resultados sin Middleware guardados en resumen_solo.csv")
    print("\n📊 Overhead guardado en resumen_overhead.csv")

if __name__ == "__main__":
    directorio_datos = "."
    procesar_csvs(directorio_datos)
