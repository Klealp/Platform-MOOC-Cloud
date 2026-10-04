import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // Salida "standalone": Next copia solo el server y las dependencias
  // estrictamente necesarias a .next/standalone, produciendo una imagen Docker
  // pequena (importante en el Web Server de 2 GiB, donde conviven api + caddy +
  // web). Sin esto habria que copiar todo node_modules.
  output: "standalone",
  // El frontend nunca expone la URL del backend al navegador: toda llamada a
  // la API de Go pasa por los Route Handlers del BFF (server-side).
  // Las imagenes de catalogo pueden venir de un CDN/almacenamiento externo.
  images: {
    remotePatterns: [
      { protocol: "https", hostname: "**" },
      { protocol: "http", hostname: "localhost" },
    ],
  },
};

export default nextConfig;
