import { lstat, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';

export default async function run({ github, context, core, exec }) {
  const config = {
    baseBranch: process.env.BASE_BRANCH || 'main',
    bootstrapSha: process.env.BOOTSTRAP_SHA || 'c500bbb758ee41a7c8a6ae382583016804a4129f',
    dryRun: process.env.DRY_RUN || 'false',
    targetRemote: process.env.TARGET_REMOTE || 'origin',
    upstreamBranch: process.env.UPSTREAM_BRANCH || 'main',
    upstreamRemote: process.env.UPSTREAM_REMOTE || 'upstream',
    upstreamRepo: process.env.UPSTREAM_REPO || 'goharbor/harbor',
    repo: process.env.GH_REPO || process.env.GITHUB_REPOSITORY || '',
    gitUserEmail: process.env.GIT_USER_EMAIL || '41898282+github-actions[bot]@users.noreply.github.com',
    gitUserName: process.env.GIT_USER_NAME || 'github-actions[bot]',
    patchDir: process.env.PATCH_DIR || 'upstream-patches',
    skipAuthors: (process.env.SKIP_AUTHORS ?? 'dependabot[bot]')
      .split(',')
      .map(author => author.trim())
      .filter(Boolean),
  };
  const [upstreamOwner, upstreamName] = config.upstreamRepo.split('/');

  const git = async (args, { ignoreReturnCode = false } = {}) => {
    const result = await exec.getExecOutput('git', args, {
      silent: true,
      ignoreReturnCode: true,
    });
    if (result.exitCode !== 0 && !ignoreReturnCode) {
      throw new Error(`git ${args.join(' ')} failed (${result.exitCode}): ${result.stderr}`);
    }
    return result;
  };
  const gitText = async (...args) => (await git(args)).stdout.replace(/\n+$/, '');
  const gitQuiet = async (...args) => git(args, { ignoreReturnCode: true });

  const shortSha = sha => gitText('rev-parse', '--short=9', sha);
  const isHandled = async (branch, sha) => {
    const remoteBranch = await gitQuiet('ls-remote', '--exit-code', '--heads', config.targetRemote, branch);
    if (remoteBranch.exitCode === 0) return true;

    const search = await github.rest.search.issuesAndPullRequests({
      q: `repo:${config.repo} is:pr in:body "Upstream-Commit: ${sha}"`,
    });
    if (search.data.total_count > 0) return true;

    const log = await gitText(
      'log',
      `${config.targetRemote}/${config.baseBranch}`,
      '-F',
      `--grep=${sha}`,
      '--format=%H',
      '-n',
      '1',
    );
    return Boolean(log);
  };

  const isSingleParentCommit = async sha => {
    const parents = await gitText('show', '-s', '--format=%P', sha);
    return parents !== '' && !parents.includes(' ');
  };

  const upstreamPrNumber = subject => subject.match(/\(#(\d+)\)$/)?.[1] || '';
  const stripPrSuffix = (subject, prNumber) =>
    prNumber ? subject.slice(0, -` (#${prNumber})`.length) : subject;

  const loadUpstreamPr = async prNumber => {
    if (!prNumber) return null;
    try {
      const { data: pr } = await github.rest.pulls.get({
        owner: upstreamOwner,
        repo: upstreamName,
        pull_number: Number(prNumber),
      });
      const reviews = await github.paginate(github.rest.pulls.listReviews, {
        owner: upstreamOwner,
        repo: upstreamName,
        pull_number: Number(prNumber),
      });
      const approvers = [...new Set(
        reviews
          .filter(review => review.state === 'APPROVED' && review.user?.login)
          .map(review => review.user.login),
      )].sort();
      return {
        title: pr.title || '',
        body: pr.body || '',
        author: pr.user?.login ? `@${pr.user.login}` : '',
        mergedBy: pr.merged_by?.login ? `@${pr.merged_by.login}` : '',
        approvals: approvers.map(login => `@${login}`).join(', '),
        url: pr.html_url || '',
      };
    } catch {
      return null;
    }
  };

  const trimBody = body => {
    const characters = Array.from(body);
    if (characters.length <= 4000) return `${body}\n`;
    return `${characters.slice(0, 4000).join('')}\n\n[Upstream PR body truncated.]\n`;
  };

  const buildCommitMessage = ({
    sha,
    subject,
    originalBody,
    status,
    prNumber,
    author,
    githubPatchFile,
  }) => {
    const header = prNumber
      ? `upstream: ${subject} (${config.upstreamRepo}#${prNumber})`
      : `upstream: ${subject}`;
    const trailers = [
      `Upstream-Commit: ${sha}`,
      ...(prNumber ? [`Upstream-PR: ${config.upstreamRepo}#${prNumber}`] : []),
      `Upstream-Author: ${author}`,
      `Cherry-Pick-Status: ${status}`,
      ...(githubPatchFile ? [`GitHub-Patch-File: ${githubPatchFile}`] : []),
    ];
    return `${header}\n\n${originalBody ? `${originalBody}\n\n` : ''}(cherry picked from commit ${sha})\n\n${trailers.join('\n')}\n`;
  };

  const buildPrBody = ({
    sha,
    upstreamShort,
    prNumber,
    title,
    author,
    mergedBy,
    approvals,
    upstreamUrl,
    status,
    description,
    githubPatchFile,
  }) => {
    const runUrl = `${process.env.GITHUB_SERVER_URL || 'https://github.com'}/${config.repo}/actions/runs/${process.env.GITHUB_RUN_ID || 'local'}`;
    const conflictBanner = status === 'conflicted'
      ? '> This cherry-pick had conflicts and was opened as a draft. Conflict markers may be present in the diff and must be resolved before marking ready for review.\n\n'
      : '';
    const summary = prNumber
      ? `Cherry-picks upstream Harbor commit \`${upstreamShort}\` from ${config.upstreamRepo}#${prNumber}.`
      : `Cherry-picks upstream Harbor commit \`${upstreamShort}\`.`;
    const upstreamPr = prNumber ? `${config.upstreamRepo}#${prNumber}` : 'unavailable';
    const patchStatus = githubPatchFile ? `- GitHub folder patch: \`${githubPatchFile}\`\n` : '';
    const upstreamDescription = description
      ? trimBody(description)
      : 'No upstream PR description available.\n';
    const patchNotes = githubPatchFile
      ? `- Changes under \`.github/\` were written to \`${githubPatchFile}\` because \`GITHUB_TOKEN\` cannot push workflow file updates.\n- Apply the patch with \`git apply ${githubPatchFile}\` if those \`.github/\` changes should be carried forward.\n`
      : '';

    return `${conflictBanner}## Summary

${summary}

## Upstream Context

- Upstream PR: ${upstreamPr}
- Upstream commit: ${sha}
- Upstream title: ${title}
- Upstream author: ${author}
- Upstream merged by: ${mergedBy || 'unavailable'}
- Upstream approved by: ${approvals || 'unavailable'}
${upstreamUrl ? `- Upstream PR URL: ${upstreamUrl}\n` : ''}- Upstream commit URL: https://github.com/${config.upstreamRepo}/commit/${sha}

## Cherry-Pick Status

- Status: ${status}
- Base branch: ${config.baseBranch}
${patchStatus}- Workflow run: ${runUrl}

## Upstream Description

${upstreamDescription}
## Review Notes

- Generated by the upstream cherry-pick workflow.
- One upstream commit maps to one Harbor Next PR.
${patchNotes}- If this PR is closed without merge, the workflow will not recreate it.

Upstream-Commit: ${sha}
${prNumber ? `Upstream-PR: ${config.upstreamRepo}#${prNumber}\n` : ''}Cherry-Pick-Status: ${status}
${githubPatchFile ? `GitHub-Patch-File: ${githubPatchFile}\n` : ''}`;
  };

  const stageCherryPickPaths = async sha => {
    await git(['add', '-u']);
    const { stdout } = await git(['diff-tree', '--no-commit-id', '--name-only', '-r', '-z', sha]);
    for (const file of stdout.split('\0').filter(Boolean)) {
      try {
        await lstat(file);
        await git(['add', '--', file]);
      } catch (error) {
        if (error.code !== 'ENOENT') throw error;
      }
    }
  };

  const moveGithubChangesToPatch = async patchFile => {
    const diffStatus = await gitQuiet('diff', '--cached', '--quiet', '--', '.github');
    if (diffStatus.exitCode === 0) return false;
    if (diffStatus.exitCode !== 1) {
      throw new Error(`git diff --cached failed (${diffStatus.exitCode}): ${diffStatus.stderr}`);
    }

    await mkdir(path.dirname(patchFile), { recursive: true });
    const { stdout } = await git(['diff', '--cached', '--binary', '--', '.github']);
    await writeFile(patchFile, stdout);
    await gitQuiet('restore', '--staged', '--worktree', '--', '.github');
    await gitQuiet('clean', '-fd', '--', '.github');
    await git(['add', patchFile]);
    return true;
  };

  const openPr = async (branch, title, body, status) => {
    if (config.dryRun === 'true') {
      core.info(`dry-run: would push ${branch} and open PR: ${title}`);
      return;
    }

    await git(['push', '--force', config.targetRemote, `HEAD:${branch}`]);
    const { data: pr } = await github.rest.pulls.create({
      owner: context.repo.owner,
      repo: context.repo.repo,
      base: config.baseBranch,
      head: branch,
      title,
      body,
      draft: status === 'conflicted',
    });
    for (const label of [
      'upstream',
      'automated',
      ...(status === 'conflicted' ? ['needs-conflict-resolution'] : []),
    ]) {
      try {
        await github.rest.issues.addLabels({
          owner: context.repo.owner,
          repo: context.repo.repo,
          issue_number: pr.number,
          labels: [label],
        });
      } catch {
        // Labels are best-effort, as in the original workflow.
      }
    }
    core.info(`opened ${pr.html_url}`);
  };

  const cherryPickCommit = async sha => {
    const subject = await gitText('show', '-s', '--format=%s', sha);
    const prNumber = upstreamPrNumber(subject);
    let upstreamSubject = stripPrSuffix(subject, prNumber);
    const upstreamShort = await shortSha(sha);
    const branch = `upstream/cherry-pick-${prNumber || 'no-pr'}-${upstreamShort}`;

    if (await isHandled(branch, sha)) {
      core.info(`handled: ${sha}`);
      return;
    }

    const upstreamPr = await loadUpstreamPr(prNumber);
    const gitAuthor = await gitText('show', '-s', '--format=%an <%ae>', sha);
    const upstreamAuthor = upstreamPr?.author || gitAuthor;
    upstreamSubject = upstreamPr?.title || upstreamSubject;
    const prTitle = prNumber
      ? `upstream: ${upstreamSubject} (${config.upstreamRepo}#${prNumber})`
      : `upstream: ${upstreamSubject}`;
    core.info(`candidate: ${sha} ${upstreamSubject}`);
    await git(['switch', '-C', branch, `${config.targetRemote}/${config.baseBranch}`]);

    const pickResult = await gitQuiet('cherry-pick', '--no-commit', sha);
    const status = pickResult.exitCode === 0 ? 'clean' : 'conflicted';
    if (status === 'conflicted') await stageCherryPickPaths(sha);

    const githubPatchPath = `${config.patchDir}/cherry-pick-${prNumber || 'no-pr'}-${upstreamShort}.github.patch`;
    const hasGithubPatch = await moveGithubChangesToPatch(githubPatchPath);
    const githubPatchFile = hasGithubPatch ? githubPatchPath : '';
    const stagedDiff = await gitQuiet('diff', '--cached', '--quiet');
    if (stagedDiff.exitCode === 0) {
      core.info(`empty: ${sha}`);
      await gitQuiet('cherry-pick', '--abort');
      await gitQuiet('cherry-pick', '--quit');
      await git(['switch', '-C', config.baseBranch, `${config.targetRemote}/${config.baseBranch}`]);
      return;
    }
    if (stagedDiff.exitCode !== 1) {
      throw new Error(`git diff --cached failed (${stagedDiff.exitCode}): ${stagedDiff.stderr}`);
    }

    const tempParent = process.env.RUNNER_TEMP || await gitText('rev-parse', '--git-path', 'tmp');
    await mkdir(tempParent, { recursive: true });
    const tempDir = await mkdtemp(path.join(tempParent, 'upstream-cherry-pick.'));
    try {
      const commitMessageFile = path.join(tempDir, 'commit-message');
      const prBodyFile = path.join(tempDir, 'pr-body');
      const originalBody = await gitText('show', '-s', '--format=%b', sha);
      await writeFile(commitMessageFile, buildCommitMessage({
        sha,
        subject: upstreamSubject,
        originalBody,
        status,
        prNumber,
        author: upstreamAuthor,
        githubPatchFile,
      }));
      const prBody = buildPrBody({
        sha,
        upstreamShort,
        prNumber,
        title: upstreamSubject,
        author: upstreamAuthor,
        mergedBy: upstreamPr?.mergedBy || '',
        approvals: upstreamPr?.approvals || '',
        upstreamUrl: upstreamPr?.url || '',
        status,
        description: upstreamPr?.body || '',
        githubPatchFile,
      });
      await writeFile(prBodyFile, prBody);

      await git(['commit', '-s', '-F', commitMessageFile]);
      await gitQuiet('cherry-pick', '--quit');
      await openPr(branch, prTitle, prBody, status);
    } finally {
      await rm(tempDir, { recursive: true, force: true });
    }
    await git(['switch', '-C', config.baseBranch, `${config.targetRemote}/${config.baseBranch}`]);
  };

  try {
    if (!config.repo) throw new Error('GH_REPO or GITHUB_REPOSITORY is required.');

    await git(['config', 'user.name', config.gitUserName]);
    await git(['config', 'user.email', config.gitUserEmail]);
    const remote = await gitQuiet('remote', 'get-url', config.upstreamRemote);
    if (remote.exitCode === 0) {
      await git(['remote', 'set-url', config.upstreamRemote, `https://github.com/${config.upstreamRepo}.git`]);
    } else {
      await git(['remote', 'add', config.upstreamRemote, `https://github.com/${config.upstreamRepo}.git`]);
    }

    await git(['fetch', config.targetRemote, config.baseBranch, '--prune']);
    await git(['fetch', config.upstreamRemote, config.upstreamBranch, '--prune']);
    await git(['switch', '-C', config.baseBranch, `${config.targetRemote}/${config.baseBranch}`]);

    const candidatesOutput = await gitText(
      'cherry',
      '-v',
      `${config.targetRemote}/${config.baseBranch}`,
      `${config.upstreamRemote}/${config.upstreamBranch}`,
      config.bootstrapSha,
    );
    const candidates = candidatesOutput
      .split('\n')
      .filter(line => line.startsWith('+'))
      .map(line => line.split(/\s+/)[1]);
    core.info(`candidates: ${candidates.length}`);

    for (const sha of candidates) {
      if (!await isSingleParentCommit(sha)) {
        core.info(`skip merge commit: ${sha}`);
        continue;
      }
      const author = await gitText('show', '-s', '--format=%an', sha);
      if (config.skipAuthors.includes(author)) {
        const subject = await gitText('show', '-s', '--format=%s', sha);
        core.info(`skip author: ${sha} ${subject}`);
        continue;
      }
      await cherryPickCommit(sha);
    }
  } catch (error) {
    core.setFailed(error.message);
  }
}
