package main

// @title WhatsApp Media Converter API
// @version 1.0.0
// @description High-performance media conversion API delivering WhatsApp-ready audio and images.
// @description
// @description **Author:** Guilherme Jansen · [GitHub](https://github.com/guilhermejansen)
// @description **Contato direto:** suporte@setupautomatizado.com.br
// @description **Coleção Postman pronta:** [Download](./swagger/postman.json)
// @description
// @description Use os exemplos documentados em cada endpoint para testar rapidamente conversões de áudio, imagem e operações S3.
// @contact.name Guilherme Jansen
// @contact.url https://github.com/guilhermejansen
// @contact.email suporte@setupautomatizado.com.br
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @BasePath /

import (
	"log"
	"os"

	docs "whats-convert-api/docs"
	"whats-convert-api/internal/config"
	"whats-convert-api/internal/server"
)

func init() {
	docs.SwaggerInfo.Description = `High-performance media conversion API delivering WhatsApp-ready audio and images.<br><br>
<strong>Autor:</strong> Guilherme Jansen · <a href="https://github.com/guilhermejansen" target="_blank">GitHub</a><br>
<strong>Contato direto:</strong> <a href="mailto:suporte@setupautomatizado.com.br">suporte@setupautomatizado.com.br</a><br>
<strong>Coleção Postman pronta:</strong> <a href="/swagger/postman.json" download>Download</a><br><br>
Use os exemplos documentados em cada endpoint para testar rapidamente conversões de áudio, imagem e operações S3.`
}

func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[MediaConverter] ")

	// Load configuration
	cfg := config.Load()

	// Create server
	srv := server.New(cfg)

	// Initialize server
	if err := srv.Initialize(); err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
		os.Exit(1)
	}

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
		os.Exit(1)
	}
}
