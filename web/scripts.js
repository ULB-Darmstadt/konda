import "htmx.org";

import Alpine from "alpinejs";

// Ensure htmx is accessible globally
window.htmx = require("htmx.org");
// window.htmx.logAll();

// Add Alpine instance to window object.
window.Alpine = Alpine;

// Start Alpine.
Alpine.start();

//Uncomment this to enable NVL-based visualization
// window.neo4jNVL = require("@neo4j-nvl/base");
// window.neo4jNVLInteraction = require("@neo4j-nvl/interaction-handlers");
