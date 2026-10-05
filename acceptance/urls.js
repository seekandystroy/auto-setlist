// Where the acceptance test servers listen; shared by the Playwright config and the test fixtures.
const appPort = process.env.ACCEPTANCE_APP_PORT || '3100';
const fakesPort = process.env.ACCEPTANCE_FAKES_PORT || '3101';

export const appURL = `http://127.0.0.1:${appPort}`;
export const fakesURL = `http://127.0.0.1:${fakesPort}`;
export { appPort, fakesPort };
