import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
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
