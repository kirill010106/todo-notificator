const express = require('express');
const swaggerUi = require('swagger-ui-express');
const YAML = require('yamljs');
const path = require('path');

const app = express();
const port = 8080;

// Загружаем наш openapi.yaml
const swaggerDocument = YAML.load(path.join(__dirname, 'openapi.yaml'));

// Подключаем UI по адресу /
app.use('/', swaggerUi.serve, swaggerUi.setup(swaggerDocument));

app.listen(port, () => {
    console.log(`Swagger UI запущен! Откройте браузер по ссылке: http://localhost:${port}`);
});
