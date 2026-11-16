# dockerfile-pin

[English](product.md)

## 概要

dockerfile-pinは、Dockerfile、composeファイル、GitHub Actionsのファイル、GitLab CIのファイルに書かれたDockerイメージの参照へ`@sha256:<digest>`を付けるコマンドラインツールです。
`run`は各タグがレジストリで今指しているdigestを調べてタグの後ろに書き、`check`はdigestのない参照と、digestがレジストリに存在しない参照を報告します。
開発者の手元やCIで動き、レジストリの認証情報は`~/.docker/config.json`から読みます。

## 背景と課題

### 背景

未確認です。未決事項を参照してください。

### 解決したい課題

未確認です。未決事項を参照してください。

## 対象ユーザー

### 未確認

- **特徴:** 未確認です。未決事項を参照してください。
- **課題:** 未確認です。未決事項を参照してください。
- **利用場面:** 未確認です。未決事項を参照してください。

## 提供価値

### 未確認

- **受け取る人:** 未確認です。未決事項を参照してください。
- **提供するもの:** 未確認です。未決事項を参照してください。
- **理由:** 未確認です。未決事項を参照してください。

## 主要な概念

- **イメージ参照:** ファイルの中でDockerイメージを指す文字列です。`node:20`、`ghcr.io/org/app:v1`、`docker://alpine:3`などで、イメージ名と、省略できるタグとdigestからなります。
- **digest:** レジストリで1つのイメージの内容を識別する`sha256:<hex>`の値です。digest付きの参照は`image:tag@sha256:<hex>`と書きます。
- **pin（固定）:** タグを残したまま、タグが今指しているdigestを参照に付けることです。
- **対象ファイル:** dockerfile-pinが読むファイルです。種類はDockerfile、composeファイル、GitHub Actionsのファイル（ワークフローか`action.yml`）、GitLab CIのファイルのいずれかで、パスから決まります。
- **スキップ:** 読み取ったがpinもcheckもしない参照です。`scratch image`、`has build directive`、`contains CI variable`などの理由が付きます。
- **除外パターン:** `--ignore-images`か`.dockerfile-pin.yaml`で指定するglobパターンです。一致したイメージをpinとcheckの対象から外します。

## 機能の一覧

| 機能 | 説明 | 仕様 |
| --- | --- | --- |
| Dockerfileのイメージ参照 | `FROM`と`COPY --from=`からイメージ参照を読み取り、digestを書き込む | 未作成 |
| YAMLのイメージ参照 | composeファイル、GitHub Actionsのファイル、GitLab CIのファイルからイメージ参照を読み取り、digestを書き込む | [Requirements](features/yaml-image-references/requirements.ja.md) |
| 対象ファイルの探索 | `-f`、`--glob`、既定のパターンから対象ファイルを探し、各ファイルの種類を決める | 未作成 |
| pin | `run`がタグをdigestに解決し、pinしたファイルを表示するか書き込む | 未作成 |
| digestの更新 | `run --update`が既存のdigestを解決し直し、`--min-age`が最近ビルドされたイメージを飛ばす | 未作成 |
| check | `check`がdigestのない参照、レジストリにないdigest、`--verify-tag`では別のdigestを指すタグを報告する | 未作成 |
| 除外と設定 | パターンでイメージを除外し、`.dockerfile-pin.yaml`から設定を読む | 未作成 |

## 成功の定義

未確認です。未決事項を参照してください。

## 開発の原則

未確認です。未決事項を参照してください。

## SLA・SLI・SLO

### SLA

ありません。dockerfile-pinは利用者の手元やCIで動くコマンドラインツールで、可用性を保証できるような提供中のサービスがないためです。

### SLIとSLO

同じ理由でありません。利用状況のデータを集めていないので、測定している指標もありません。

## 未決事項

### 目的・利用者・提供価値

- **確認したいこと:** 次の目的・利用者・提供価値が、今もdockerfile-pinに当てはまるか。
- **候補とその根拠:** タグは書き換えられるので、同じタグが時期によって別のイメージを指すことがあり、digestでpinするとビルドはレビューしたイメージのままになる。pinactがGitHub Actionsで行うのと同じように、サプライチェーン攻撃を防ぐことが目的。利用者はDockerfile・composeファイル・CIのファイルを管理する開発者で、移行のために手元で、検査のためにCIで使う。根拠はREADMEの最初の段落と、`docs/superpowers/specs/2026-03-27-dockerpin-design.md`のMotivationとGoals。
- **決まるまで:** 背景、課題、対象ユーザー、提供価値、成功の定義、開発の原則は、本文では未確認のままにする。
- **判断する人:** メンテナー（azu）。

### Renovateとの関係でのdigestの更新

- **確認したいこと:** 2026-03-27の設計ではdigestの自動更新をRenovateに任せる対象外の項目にしているが、今は`run --update`と`--min-age`がある。dockerfile-pinがdigestの更新も担うのか、継続的な更新は引き続きRenovateに任せるのか。
- **候補とその根拠:** READMEは継続的な自動更新にはRenovateを勧め、`--update`を手動で解決し直す機能として説明している。
- **決まるまで:** digestの更新の機能は、Renovateとの関係を書かずに一覧へ載せる。
- **判断する人:** メンテナー（azu）。
