import { openapi } from '@nestjs/swagger';

const API_SCHEMA = openapi({
  info: {
    title: 'Goal API',
    description: 'Goal API Documentation',
    version: '1.0.0',
  },
  servers: [
    {
      url: 'https://goal-api.com',
      description: 'Production environment',
    },
  ],
  paths: {
    '/goals': {
      get: {
        summary: 'Get all goals',
        responses: {
          200: {
            description: 'List of goals',
            content: {
              'application/json': {
                schema: {
                  $ref: '#/components/schemas/Goal',
                },
              },
            },
          },
        },
      },
      post: {
        summary: 'Create a new goal',
        requestBody: {
          content: {
            'application/json': {
              schema: {
                $ref: '#/components/schemas/GoalCreate',
              },
            },
          },
        },
        responses: {
          201: {
            description: 'Created goal',
            content: {
              'application/json': {
                schema: {
                  $ref: '#/components/schemas/Goal',
                },
              },
            },
          },
        },
      },
    },
  },
  components: {
    schemas: {
      Goal: {
        type: 'object',
        properties: {
          id: {
            type: 'integer',
            format: 'int32',
          },
          title: {
            type: 'string',
          },
          description: {
            type: 'string',
          },
        },
      },
      GoalCreate: {
        type: 'object',
        properties: {
          title: {
            type: 'string',
          },
          description: {
            type: 'string',
          },
        },
      },
    },
  },
});

export { API_SCHEMA };