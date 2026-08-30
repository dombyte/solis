import SwaggerUIBundle from 'swagger-ui-dist/swagger-ui-bundle';
import SwaggerUIStandalonePreset from 'swagger-ui-dist/swagger-ui-standalone-preset';
import 'swagger-ui-dist/swagger-ui.css';
import spec from './openapi.yaml';

// Initialize when DOM is ready
if (document.readyState === 'complete' || document.readyState === 'interactive') {
  setTimeout(initSwagger);
} else {
  document.addEventListener('DOMContentLoaded', initSwagger);
}

function initSwagger() {
  const ui = SwaggerUIBundle({
    spec,
    dom_id: '#swagger',
    presets: [
      SwaggerUIBundle.presets.apis,
      SwaggerUIStandalonePreset
    ],
    // Use BaseLayout instead of StandaloneLayout to remove top bar with search
    layout: "BaseLayout",
    deepLinking: true,
    tryItOutEnabled: true,
    docExpansion: "list",
    defaultModelsExpandDepth: 1,
    defaultModelExpandDepth: 1,
    showExtensions: false,
    showCommonExtensions: false
  });
  window.ui = ui;
}
