一个更复杂的 DAG 场景：4层、多个并行分支、多对多依赖。

        alpine (L1)
        /      \
step2-1   step2-2   (L2, 并行)
    |    \  /    |
    |   step2-3  |    (L2, 也在第2层，无前驱依赖alpine)
    |       |    |
step3-1  step3-2   (L3, 并行，step3-1依赖step2-1+step2-3，step3-2依赖step2-2+step2-3)
        \      /
        step4         (L4)

实际上 step2-3 也依赖 alpine，所以 L2 = [step2-1, step2-2, step2-3]。