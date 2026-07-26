import { readFile } from 'node:fs/promises'

const contractURL = new URL('../api/openapi/openapi.json', import.meta.url)
const contract = JSON.parse(await readFile(contractURL, 'utf8'))

const requiredPaths = [
  '/healthz',
  '/readyz',
  '/api/v1',
  '/api/v1/projects',
  '/api/v1/projects/{slug}',
  '/api/v1/projects/{slug}/documents',
  '/api/v1/projects/{slug}/documents/{documentSlug}',
  '/api/v1/projects/{slug}/documents/{documentSlug}/comments',
]

if (contract.openapi !== '3.1.0') {
  throw new Error(`expected OpenAPI 3.1.0, got ${contract.openapi}`)
}

for (const path of requiredPaths) {
  if (!contract.paths?.[path]?.get?.responses?.['200']) {
    throw new Error(`missing GET 200 response for ${path}`)
  }
}

const requiredOperations = [
  ['post', '/api/v1/projects/{slug}/documents/{documentSlug}/comments', '201'],
  ['patch', '/api/v1/projects/{slug}/documents/{documentSlug}/comments/{commentID}', '200'],
]

for (const [method, path, status] of requiredOperations) {
  if (!contract.paths?.[path]?.[method]?.responses?.[status]) {
    throw new Error(`missing ${method.toUpperCase()} ${status} response for ${path}`)
  }
}

const schemas = contract.components?.schemas ?? {}
const responses = contract.components?.responses ?? {}

function validateReferences(value, location = '#') {
  if (Array.isArray(value)) {
    value.forEach((item, index) => validateReferences(item, `${location}/${index}`))
    return
  }
  if (!value || typeof value !== 'object') return

  if (typeof value.$ref === 'string') {
    const schemaPrefix = '#/components/schemas/'
    const responsePrefix = '#/components/responses/'
    if (value.$ref.startsWith(schemaPrefix) && !schemas[value.$ref.slice(schemaPrefix.length)]) {
      throw new Error(`unresolved schema reference ${value.$ref} at ${location}`)
    }
    if (value.$ref.startsWith(responsePrefix) && !responses[value.$ref.slice(responsePrefix.length)]) {
      throw new Error(`unresolved response reference ${value.$ref} at ${location}`)
    }
  }

  for (const [key, child] of Object.entries(value)) {
    validateReferences(child, `${location}/${key}`)
  }
}

validateReferences(contract)
console.log(`OpenAPI contract valid: ${Object.keys(contract.paths).length} paths, ${Object.keys(schemas).length} schemas`)
