# Dockerfileのイメージ参照の設計

[English](design.md)

## この文書について

この設計は、[dockerfile-pin](../../product.ja.md)で、[Dockerfileのイメージ参照](requirements.ja.md)が定める参照をどう読み取り、どう書き換えるかを説明します。

### 想定読者

Dockerfileの読み取りや書き換えを変える人とエージェント。

### 前提知識

Dockerfileの構文、マルチステージビルド、BuildKitが`FROM`、`COPY --from`、`RUN --mount`を解決する方法。

### この文書で扱わないこと

ファイルの探索、digestの解決、コマンドの出力は、それぞれの機能に属します。

## 実装方針

`internal/dockerfile`には、読み取りの`Parse`と書き換えの`RewriteFileReport`があります。
`run`と`check`は`Parse`を呼び、各参照をどう扱うかを決めます。`run`は選んだdigestを書き換えに渡します。

### 読み取り

`Parse`はBuildKitのDockerfileパーサー（`github.com/moby/buildkit/frontend/dockerfile/parser`）を使い、トップレベルの命令を順にたどります。
`docker build`と同じパーサーを使うことで、Dockerfileの構文を実装し直さずに、命令がまたがる行、行継続をつないだ文字列、フラグ、escapeの文字を得られます。

名前をどう分類するかは、3つの状態で決まります。
`ARG`の既定値は、たどりながら集めます。そのため`FROM`からは、Dockerのスコープと同じく、自分より上の`ARG`だけが見えます。
`FROM`のために、それまでに見たステージ名も同じように集めます。BuildKitは各`FROM`を、それより上で宣言されたステージに対してだけ解決するためです。
`COPY --from`のためには、たどる前にファイル内のすべてのステージ名を集めます。後で宣言されるステージを使うのはステージの順序についてのビルドエラーで、pinしていないイメージではないので、pinするものがありません。

`ONBUILD`は、包んだ命令を、位置が設定されていない子ノードにぶら下げます。そのため読み取りは、`ONBUILD`のノードの行の範囲と文字列を子に写し、ステージ名を渡さずに分類します。

分類はBuildKitと同じ順序で行います。`scratch`、値全体でのステージ名との完全一致、最後の`@`でのdigestの分離の順です。
`COPY --from`では、ステージの番号（`strconv.Atoi`が受け付けるもの）と、値の中の`$`を先に調べます。BuildKitはこの値をそのまま読み、変数を展開しないためです（[moby/buildkit#2374](https://github.com/moby/buildkit/issues/2374)）。

`RUN`では、パーサーが先頭の各フラグを引用符を外した形で返します。読み取りは`--mount=`で始まるフラグを取り出し、それぞれの値を1件のCSVレコードとして分け、最後の`from`オプションの値を使います。キーは、BuildKitの`parseMount`と同じく小文字にして比べます（[commands_runmount.go](https://github.com/moby/buildkit/blob/v0.16.0/frontend/dockerfile/instructions/commands_runmount.go)）。
同じ関数は、`from`の中の`$`を`'from' doesn't support variable expansion, define alias stage instead`というエラーで拒否します。そのような値をスキップするのはこのためです。
その後、値を`COPY --from`と同じように分類しますが、ステージの番号の確認はしません。BuildKitはマウント元に対して`findStateByName`だけを呼び、見つからなければ値をイメージ名としてステージを作るためです（[convert_runmount.go](https://github.com/moby/buildkit/blob/v0.16.0/frontend/dockerfile/dockerfile2llb/convert_runmount.go)）。
ステージ名は`COPY --from`と同じく、ファイル全体のものを使います。BuildKitはマウント元もすべてのステージから探し、後ろで宣言されたステージはステージの順序のエラーとして報告するためです。
各参照には、その`--mount`フラグが命令の`--mount`フラグの中で何番目かを記録します。書き換えがこれを必要とします。

### 書き換え

書き換えは、ファイルを行として扱い、その場で編集します。
選んだ参照ごとに、命令がまたがる行をパーサーと同じ方法でつなぎます。続く行ではescapeの文字とその後ろを落とし、つないだ文字列の各バイトが元のどの行のどの列から来たかを記録します。
つないだ文字列から参照を探します。`COPY`では`--from=`を前に付けて探します。見つけた位置を行へ戻し、既存のdigestのバイトを取り除き、digestを除いた参照が終わる場所に`@<digest>`を挿入します。
`RUN --mount`では、参照を探すのではなく、文字列を読んで場所を決めます。
書き換えは命令を単語に分けます。引用符の中の空白は単語の一部として扱います。命令名の後ろから、`--`で始まらない最初の単語の手前までがフラグで、その中の`--mount=`フラグのうち、記録した番号のものを変えます。
そのフラグを引用符の外のカンマで分け、キーが大文字と小文字を区別せずに`from`である最後のオプションを選び、その値にdigestを書きます。値が引用符で始まるときは、その後ろから書きます。
`from=<参照>`を探す方法では、3つの場合に誤ります。同じイメージを指す2つのマウントがあると、最初のマウントを2回見つけます。`from`が繰り返されていると、BuildKitは最後のものを使うのに、最初のものをpinします。`source=`のようなほかのオプションの中に同じ文字列があると、そちらを変えます。
つないだ文字列を探すので、行継続で分かれた参照も見つかります。物理的な行ごとに探すと見落とし、ファイルが気付かないうちに変わらないままになります。

参照が見つからないときは、書き換えできなかった参照の一覧にその添字を入れて返します。`run`はこれを使って警告し、数から外します。

## 構成図とシーケンス図

図は必要ありません。流れは`Parse`、呼び出し側でのdigestの決定、`RewriteFileReport`の順で、YAMLの読み取りと同じ形です。

## データモデルとER図

`FromInstruction`は、`RawRef`、`ImageRef`、`Digest`、`Platform`、`StageName`、`StartLine`、`EndLine`、`Original`、`Skip`、`SkipReason`、`IsCopyFrom`、`IsMountFrom`、`MountIndex`、`EscapeToken`を持ちます。
スキップの理由は公開された定数にしてあり、呼び出し側とテストが文字列を繰り返さずに済みます。

## APIとインターフェース

`Parse(r io.Reader)`は、参照か、パーサーのエラーを返します。
`RewriteFileReport(content, instructions, digests)`は、新しい内容と、書き換えできなかった添字を返します。`digests`のキーは参照の添字です。`RewriteFile`は、一覧を返さない同じ関数です。

## 外部サービスとの連携

ありません。BuildKitのパーサーはライブラリとしての依存で、`FROM`の行を正確に解析するために選んでいます。

## データの取り扱い

参照はコマンドの実行中にメモリーにあるだけです。

## 認証・認可の注意点

ありません。ローカルのファイルを読み書きするだけです。

## セキュリティ上の考慮事項

各規則は推測ではなく、BuildKit自身の命令の読み方に合わせています。Dockerがレジストリから解決する行を、ステージとして分類してスキップすることがないようにするためです。

## デプロイ時の注意点

ありません。

## 検討した代替手段

### `COPY --from`の変数を展開する

`COPY --from=image:${TAG}`を、変数が指すイメージにpinできます。
BuildKitはこの値を展開せずビルドが失敗するので、ビルドできない行にdigestを書くことになり、`check`はその行をOKと報告してしまいます。採用しません（[#54](https://github.com/azu/dockerfile-pin/pull/54)）。

### BuildKitの`instructions`パッケージでマウントを読む

`instructions.Parse`を使えば、各マウントを解析済みの形で、`From`フィールド付きで得られます。
ただしDockerfile全体を検証し、受け付けない命令があると失敗します。書き換えに必要な行の中の位置も得られません。そのため、すでに使っているパーサーが返すフラグを読みます。

### `from=`の数値をステージの番号として扱う

`COPY --from=0`とそろいます。しかしBuildKitはマウント元をそのようには読まないので、ビルドがイメージとしてpullする値を`check`がスキップすることになります。採用しません。

### `FROM`と`COPY --from`で関数を分ける

呼び出し側はどちらも両方の種類を必要とし、パッケージは内部用なので、`IsCopyFrom`で区別する1つの`Parse`のままにしています。`RUN --mount`も`IsMountFrom`でここに加わります（[#54](https://github.com/azu/dockerfile-pin/pull/54)）。

## 未決事項

ありません。
