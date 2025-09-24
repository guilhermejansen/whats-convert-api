const transformCommitTimestamps = commit => {
  const normalize = key => {
    if (commit && commit[key] && /^\d+$/.test(commit[key])) {
      const milliseconds = Number(commit[key]) * 1000;
      commit[key] = new Date(milliseconds).toISOString();
    }
  };

  normalize('authorDate');
  normalize('committerDate');

  return commit;
};

module.exports = {
  branches: ['main'],
  tagFormat: 'v${version}',
  plugins: [
    [
      '@semantic-release/commit-analyzer',
      {
        preset: 'conventionalcommits'
      }
    ],
    [
      '@semantic-release/release-notes-generator',
      {
        preset: 'conventionalcommits',
        writerOpts: {
          transform: transformCommitTimestamps
        }
      }
    ],
    [
      '@semantic-release/changelog',
      {
        changelogFile: 'CHANGELOG.md'
      }
    ],
    [
      '@semantic-release/exec',
      {
        prepareCmd: 'echo ${nextRelease.version} > VERSION'
      }
    ],
    [
      '@semantic-release/git',
      {
        assets: ['CHANGELOG.md', 'VERSION'],
        message: 'chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}'
      }
    ],
    '@semantic-release/github'
  ]
};
